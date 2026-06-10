package services

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/zclconf/go-cty/cty"
)

// ManifestResourceRef 浅 parse 抽出的 resource/module 引用
//
// 仅记录 (type, name),不解析 body。for_each / count 多实例的资源在这里
// 只展示为单条(type=资源类型,name=声明名),具体实例数量与 key 以
// terraform state 为权威。
type ManifestResourceRef struct {
	Kind string // "resource" 或 "module"
	Type string // resource 类型(如 aws_vpc);module 时为空
	Name string // resource 声明名(如 main);module 时为模块实例名
	File string // 来自哪个文件(便于 UI 展示)
}

// WorkspaceResourceID 返回该 ref 落 workspace_resources 时的 resource_id。
//   - resource 块: "<type>.<name>" (如 aws_vpc.main),对齐 terraform 资源地址
//   - module 块:   "module.<name>" (如 module.network),对齐 terraform module 地址
func (r ManifestResourceRef) WorkspaceResourceID() string {
	if r.Kind == "module" {
		return "module." + r.Name
	}
	return r.Type + "." + r.Name
}

// WorkspaceResourceType 返回落表用的 resource_type。
// module 块没有真实资源类型,统一用 "module" 占位(具体 module source 由输出提示
// 端点从 manifest_files 实时解析,不冗余存这里)。
func (r ManifestResourceRef) WorkspaceResourceType() string {
	if r.Kind == "module" {
		return "module"
	}
	return r.Type
}

// ParseManifestResources 浅 parse 一组 .tf 文件,只取 top-level resource / module block 的标签。
//
// - 仅扫顶层 .tf 文件(模块内部资源不展开,那是 terraform 自己的事)
// - .tf.json / .tfvars / 二进制 / 其他扩展名 跳过
// - 半成品 HCL(块未闭合等)skip,不阻塞调用方
//
// scopeFiles 是 (path -> raw bytes) 映射,通常对应 manifest_files 拉到的内容。
// subpath 非空时只 parse 该子目录的顶层文件(模拟 terraform plan 在 subpath 跑)。
func ParseManifestResources(scopeFiles map[string][]byte, subpath string) []ManifestResourceRef {
	parser := hclparse.NewParser()
	var refs []ManifestResourceRef
	// 去重: 同一 (kind, type, name) 只保留首次出现。两个文件声明同名 resource 在
	// 单文件维度各自合法,但落 workspace_resources 时 resource_id=<type>.<name> 唯一,
	// 不去重会插入重复行,且 upgrade reconcile 的 map 会漏删(spec: best-effort 视图)。
	seen := make(map[string]bool)

	subpath = strings.TrimSuffix(subpath, "/")

	for path, content := range scopeFiles {
		if !shouldParseForResources(path, subpath) {
			continue
		}

		file, diags := parser.ParseHCL(content, path)
		if diags.HasErrors() || file == nil {
			// 半成品 HCL: skip
			continue
		}

		// PartialContent 只取我们关心的 block 类型,其他忽略
		schema := &hcl.BodySchema{
			Blocks: []hcl.BlockHeaderSchema{
				{Type: "resource", LabelNames: []string{"type", "name"}},
				{Type: "module", LabelNames: []string{"name"}},
			},
		}
		bodyContent, _, partDiags := file.Body.PartialContent(schema)
		if partDiags.HasErrors() || bodyContent == nil {
			continue
		}

		for _, block := range bodyContent.Blocks {
			switch block.Type {
			case "resource":
				if len(block.Labels) >= 2 {
					key := "resource\x00" + block.Labels[0] + "\x00" + block.Labels[1]
					if seen[key] {
						continue
					}
					seen[key] = true
					refs = append(refs, ManifestResourceRef{
						Kind: "resource",
						Type: block.Labels[0],
						Name: block.Labels[1],
						File: path,
					})
				}
			case "module":
				if len(block.Labels) >= 1 {
					key := "module\x00\x00" + block.Labels[0]
					if seen[key] {
						continue
					}
					seen[key] = true
					refs = append(refs, ManifestResourceRef{
						Kind: "module",
						Type: "",
						Name: block.Labels[0],
						File: path,
					})
				}
			}
		}
	}

	return refs
}

// ParseManifestModuleSources 浅 parse 一组 .tf 文件,提取顶层 module block 的
// (instance_name -> source) 映射。供 workspace 输出提示把 manifest-managed 资源
// (无 tf_code)映射回平台 module → schema → outputs。
//
// 只取 subpath 直接下层 .tf(与执行/解析 scope 一致);source 取字面量字符串,
// 非字面量(引用变量等)跳过;半成品 HCL skip。同名 module 取首个。
func ParseManifestModuleSources(scopeFiles map[string][]byte, subpath string) map[string]string {
	return parseManifestModuleSources(scopeFiles, func(path string) bool {
		return shouldParseForResources(path, subpath)
	})
}

// ParseManifestModuleSourcesForCheck 提取本次 manifest check 提交内容里的 module source。
//
// 与 ParseManifestModuleSources 不同,check 的输入已经是前端明确提交的待检查文件,
// 文件 path 可能带目录;这里不再套用 terraform 执行目录的顶层过滤,避免漏召回实际
// 被检查文件引用的平台 module skill。
func ParseManifestModuleSourcesForCheck(scopeFiles map[string][]byte) map[string]string {
	return parseManifestModuleSources(scopeFiles, func(path string) bool {
		return filepath.Ext(path) == ".tf"
	})
}

func parseManifestModuleSources(scopeFiles map[string][]byte, shouldParse func(string) bool) map[string]string {
	parser := hclparse.NewParser()
	out := make(map[string]string)

	for path, content := range scopeFiles {
		if !shouldParse(path) {
			continue
		}
		file, diags := parser.ParseHCL(content, path)
		if diags.HasErrors() || file == nil {
			continue
		}
		schema := &hcl.BodySchema{
			Blocks: []hcl.BlockHeaderSchema{
				{Type: "module", LabelNames: []string{"name"}},
			},
		}
		bodyContent, _, partDiags := file.Body.PartialContent(schema)
		if partDiags.HasErrors() || bodyContent == nil {
			continue
		}
		for _, block := range bodyContent.Blocks {
			if block.Type != "module" || len(block.Labels) < 1 {
				continue
			}
			name := block.Labels[0]
			if _, ok := out[name]; ok {
				continue
			}
			attrContent, _, _ := block.Body.PartialContent(&hcl.BodySchema{
				Attributes: []hcl.AttributeSchema{{Name: "source"}},
			})
			if attrContent == nil {
				continue
			}
			if a, ok := attrContent.Attributes["source"]; ok {
				if v, vdiags := a.Expr.Value(nil); !vdiags.HasErrors() && v.Type() == cty.String {
					out[name] = v.AsString()
				}
			}
		}
	}
	return out
}

// ManifestVariableMeta 从 variable block 浅 parse 出的 input variable 元信息
//
// 平台不维护类型系统(workspace/varset 变量都是 key->value),这里同样不做 cty 求值:
// TypeRaw / DefaultRaw 仅存 HCL 表达式的原始源码字符串供 UI 展示。
type ManifestVariableMeta struct {
	Name        string `json:"name"`                  // 变量名
	Description string `json:"description,omitempty"` // description = "..." 的字面量
	Required    bool   `json:"required"`              // 无 default 即 required
	Sensitive   bool   `json:"sensitive,omitempty"`   // sensitive = true
	TypeRaw     string `json:"type_raw,omitempty"`    // type 表达式原始源码,如 "string" / "list(string)"
	DefaultRaw  string `json:"default_raw,omitempty"` // default 表达式原始源码;无 default 则空
}

// ParseManifestVariables 浅 parse 一组 .tf 文件,提取顶层 variable block 的元信息。
//
//   - 仅扫根目录顶层 .tf(与 terraform 默认行为一致,不递归子目录;manifest 发布时
//     还不知道 deployment 的 subpath,故固定按根目录全部顶层 .tf 取并集)
//   - type / default 只取原始源码字符串,不做类型求值(复杂表达式也不会解析失败)
//   - description / sensitive 尝试取字面量,取不到则留默认值
//   - 半成品 HCL(块未闭合等)skip,不阻塞调用方
//
// scopeFiles 是 (path -> raw bytes) 映射,通常对应 manifest_files 拉到的内容。
func ParseManifestVariables(scopeFiles map[string][]byte) []ManifestVariableMeta {
	parser := hclparse.NewParser()
	var metas []ManifestVariableMeta
	seen := make(map[string]bool) // 同名变量去重(多文件声明只取首个)

	for path, content := range scopeFiles {
		// 复用 resource 的 scope 规则(根目录顶层 .tf),subpath 传空 = 根目录
		if !shouldParseForResources(path, "") {
			continue
		}

		file, diags := parser.ParseHCL(content, path)
		if diags.HasErrors() || file == nil {
			continue
		}

		schema := &hcl.BodySchema{
			Blocks: []hcl.BlockHeaderSchema{
				{Type: "variable", LabelNames: []string{"name"}},
			},
		}
		bodyContent, _, partDiags := file.Body.PartialContent(schema)
		if partDiags.HasErrors() || bodyContent == nil {
			continue
		}

		for _, block := range bodyContent.Blocks {
			if block.Type != "variable" || len(block.Labels) < 1 {
				continue
			}
			name := block.Labels[0]
			if seen[name] {
				continue
			}
			seen[name] = true

			meta := ManifestVariableMeta{Name: name, Required: true}

			// variable block body 里关心的属性
			attrSchema := &hcl.BodySchema{
				Attributes: []hcl.AttributeSchema{
					{Name: "type"},
					{Name: "description"},
					{Name: "default"},
					{Name: "sensitive"},
				},
			}
			attrContent, _, _ := block.Body.PartialContent(attrSchema)
			if attrContent == nil {
				metas = append(metas, meta)
				continue
			}

			if a, ok := attrContent.Attributes["type"]; ok {
				meta.TypeRaw = rawExprSource(content, a.Expr)
			}
			if a, ok := attrContent.Attributes["default"]; ok {
				meta.DefaultRaw = rawExprSource(content, a.Expr)
				meta.Required = false // 有 default 即非必填
			}
			if a, ok := attrContent.Attributes["description"]; ok {
				if v, vdiags := a.Expr.Value(nil); !vdiags.HasErrors() && v.Type() == cty.String {
					meta.Description = v.AsString()
				}
			}
			if a, ok := attrContent.Attributes["sensitive"]; ok {
				if v, vdiags := a.Expr.Value(nil); !vdiags.HasErrors() && v.Type() == cty.Bool {
					meta.Sensitive = v.True()
				}
			}

			metas = append(metas, meta)
		}
	}

	return metas
}

// rawExprSource 用表达式的源码区间从原始内容里切出原文,去掉首尾空白。
// 取不到(range 越界等)时退回空串。
func rawExprSource(content []byte, expr hcl.Expression) string {
	rng := expr.Range()
	start, end := rng.Start.Byte, rng.End.Byte
	if start < 0 || end > len(content) || start >= end {
		return ""
	}
	return strings.TrimSpace(string(content[start:end]))
}

// IsTopLevelTFUnderSubpath 判断 path 是否为 subpath 直接下层(非递归)的 .tf 文件。
// 导出供 deployment install 的 subpath 存在性校验复用,确保校验范围与实际解析/执行一致。
func IsTopLevelTFUnderSubpath(path, subpath string) bool {
	return shouldParseForResources(path, subpath)
}

// shouldParseForResources 仅顶层 .tf 文件 (在 subpath 下) 才扫
func shouldParseForResources(path, subpath string) bool {
	// 扩展名
	if filepath.Ext(path) != ".tf" {
		return false
	}

	// 规范化路径分隔
	clean := filepath.ToSlash(filepath.Clean(path))
	if subpath != "" {
		clean = strings.TrimPrefix(clean, subpath+"/")
		if clean == path { // 不在 subpath 下
			return false
		}
	}

	// terraform 不递归子目录: 顶层 .tf 才纳入
	return !strings.Contains(clean, "/")
}

// ParsedModuleBlock 从 HCL 文本解析出的 module 块(含全部参数)。
// 用于生成后的 schema 校验:按 Source/Version 定位仓库 module,用 Parameters 跑 SchemaSolver。
type ParsedModuleBlock struct {
	InstanceName  string                 // module "xxx" 的 xxx
	Source        string                 // source 属性
	Version       string                 // version 属性(可空)
	Parameters    map[string]interface{} // 可静态求值的参数(除 source/version)
	PresentParams map[string]bool        // 块里出现过的所有参数名(含引用变量等不可静态求值的),用于必填检查
	StartLine     int                    // 块起始行(1-based)
	EndLine       int                    // 块结束行(1-based)
}

// ParseManifestModuleBlocks 从单个 HCL 文本解析所有 module 块的参数。
// Parameters 只含可静态求值的字面量;PresentParams 记录所有出现过的参数名(含 var./资源引用),
// 供 schema 必填检查区分"完全没写"和"写了但引用变量"——后者不应报缺失。

func ParseManifestModuleBlocks(content string) []ParsedModuleBlock {
	parser := hclparse.NewParser()
	file, diags := parser.ParseHCL([]byte(content), "generated.tf")
	if diags.HasErrors() || file == nil {
		return nil
	}
	bodyContent, _, partDiags := file.Body.PartialContent(&hcl.BodySchema{
		Blocks: []hcl.BlockHeaderSchema{{Type: "module", LabelNames: []string{"name"}}},
	})
	if partDiags.HasErrors() || bodyContent == nil {
		return nil
	}

	var out []ParsedModuleBlock
	for _, block := range bodyContent.Blocks {
		if block.Type != "module" || len(block.Labels) < 1 {
			continue
		}
		pmb := ParsedModuleBlock{
			InstanceName:  block.Labels[0],
			Parameters:    make(map[string]interface{}),
			PresentParams: make(map[string]bool),
		}
		if r := block.DefRange; r.Filename != "" {
			pmb.StartLine = r.Start.Line
		}
		attrs, _ := block.Body.JustAttributes()
		var maxLine int
		for name, attr := range attrs {
			if attr.Range.End.Line > maxLine {
				maxLine = attr.Range.End.Line
			}
			// 记录出现过的参数名(无论能否静态求值),source/version 不计入参数
			if name != "source" && name != "version" {
				pmb.PresentParams[name] = true
			}
			v, vdiags := attr.Expr.Value(nil)
			if vdiags.HasErrors() {
				continue // 依赖 var/资源引用,无法静态求值;仅记名,不取值
			}
			goVal := ctyToGo(v)
			switch name {
			case "source":
				if s, ok := goVal.(string); ok {
					pmb.Source = s
				}
			case "version":
				if s, ok := goVal.(string); ok {
					pmb.Version = s
				}
			default:
				pmb.Parameters[name] = goVal
			}
		}
		if maxLine > pmb.StartLine {
			pmb.EndLine = maxLine
		} else {
			pmb.EndLine = pmb.StartLine
		}
		out = append(out, pmb)
	}
	return out
}

// ctyToGo 把 cty.Value 转成 Go 原生值(string/float64/bool/[]interface{}/map[string]interface{})。
func ctyToGo(v cty.Value) interface{} {
	if v.IsNull() || !v.IsKnown() {
		return nil
	}
	t := v.Type()
	switch {
	case t == cty.String:
		return v.AsString()
	case t == cty.Bool:
		return v.True()
	case t == cty.Number:
		f, _ := v.AsBigFloat().Float64()
		return f
	case t.IsTupleType() || t.IsListType() || t.IsSetType():
		var arr []interface{}
		for it := v.ElementIterator(); it.Next(); {
			_, ev := it.Element()
			arr = append(arr, ctyToGo(ev))
		}
		return arr
	case t.IsObjectType() || t.IsMapType():
		m := make(map[string]interface{})
		for it := v.ElementIterator(); it.Next(); {
			kv, ev := it.Element()
			m[kv.AsString()] = ctyToGo(ev)
		}
		return m
	default:
		return nil
	}
}

// ExtractResourceBlock 从一组 .tf 文件中提取指定 resource 或 module block 的完整 HCL 源码。
//
// kind: "resource" 或 "module"
// typeName: resource 类型（如 "aws_s3_bucket"），module 时传空
// name: 声明名（如 "my_bucket" 或 module 实例名 "network"）
// subpath: terraform 执行子目录（空 = 根目录）
//
// 返回 (文件路径, HCL 文本, 是否找到)。跨文件搜索，同名 resource 取首个匹配。
func ExtractResourceBlock(
	scopeFiles map[string][]byte,
	kind, typeName, name, subpath string,
) (filePath string, hclText string, found bool) {
	parser := hclparse.NewParser()
	subpath = strings.TrimSuffix(subpath, "/")

	// 排序确保确定性遍历顺序（同名 resource 取首个匹配时结果稳定）
	paths := make([]string, 0, len(scopeFiles))
	for p := range scopeFiles {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	for _, path := range paths {
		content := scopeFiles[path]
		if !shouldParseForResources(path, subpath) {
			continue
		}

		file, diags := parser.ParseHCL(content, path)
		if diags.HasErrors() || file == nil {
			continue
		}

		var labelNames []string
		if kind == "resource" {
			labelNames = []string{"type", "name"}
		} else {
			labelNames = []string{"name"}
		}

		schema := &hcl.BodySchema{
			Blocks: []hcl.BlockHeaderSchema{
				{Type: kind, LabelNames: labelNames},
			},
		}
		bodyContent, _, partDiags := file.Body.PartialContent(schema)
		if partDiags.HasErrors() || bodyContent == nil {
			continue
		}

		for _, block := range bodyContent.Blocks {
			if block.Type != kind {
				continue
			}
			if kind == "resource" && len(block.Labels) >= 2 {
				if block.Labels[0] != typeName || block.Labels[1] != name {
					continue
				}
			} else if kind == "module" && len(block.Labels) >= 1 {
				if block.Labels[0] != name {
					continue
				}
			} else {
				continue
			}

			// 提取完整 block 文本：从 DefRange.Start 到 body 最后一个属性/块的 End
			text := extractBlockText(content, block)
			if text == "" {
				continue
			}
			return path, text, true
		}
	}
	return "", "", false
}

// extractBlockText 从原始 HCL 内容中提取一个 block 的完整源码文本（含花括号）。
// 使用花括号配对方式定位 block 结束位置，避免依赖 HCL schema 遍历 nested blocks。
func extractBlockText(content []byte, block *hcl.Block) string {
	startByte := block.DefRange.Start.Byte
	if startByte < 0 || startByte >= len(content) {
		return ""
	}

	// 找到 block header 之后的第一个 '{'
	pos := startByte
	for pos < len(content) && content[pos] != '{' {
		pos++
	}
	if pos >= len(content) {
		return ""
	}

	// 从 '{' 开始做花括号配对，正确处理字符串和注释
	depth := 0
	inString := false
	heredocIdent := "" // 非空 = 在 heredoc 内部
	i := pos
	for i < len(content) {
		b := content[i]

		// heredoc 处理：逐行扫描，只有整行（trim 后）精确匹配 heredocIdent 才结束
		if heredocIdent != "" {
			if b == '\n' {
				// 读取下一行内容
				lineStart := i + 1
				lineEnd := lineStart
				for lineEnd < len(content) && content[lineEnd] != '\n' && content[lineEnd] != '\r' {
					lineEnd++
				}
				lineText := strings.TrimSpace(string(content[lineStart:lineEnd]))
				if lineText == heredocIdent {
					heredocIdent = ""
					i = lineEnd
					continue
				}
			}
			i++
			continue
		}

		// 字符串内：只关心转义和结束引号
		if inString {
			if b == '\\' && i+1 < len(content) {
				i += 2 // 跳过转义
				continue
			}
			if b == '"' {
				inString = false
			}
			i++
			continue
		}

		switch b {
		case '"':
			inString = true
		case '#':
			// 行注释：跳到行尾
			for i < len(content) && content[i] != '\n' {
				i++
			}
			continue
		case '/':
			if i+1 < len(content) && content[i+1] == '/' {
				for i < len(content) && content[i] != '\n' {
					i++
				}
				continue
			}
			if i+1 < len(content) && content[i+1] == '*' {
				i += 2
				for i+1 < len(content) {
					if content[i] == '*' && content[i+1] == '/' {
						i += 2
						break
					}
					i++
				}
				continue
			}
		case '<':
			// heredoc: <<IDENT 或 <<-IDENT（必须是合法标识符，否则是位移运算符）
			if i+1 < len(content) && content[i+1] == '<' {
				hStart := i + 2
				indented := false
				if hStart < len(content) && content[hStart] == '-' {
					indented = true
					hStart++
				}
				hEnd := hStart
				for hEnd < len(content) && isIdentChar(content[hEnd]) {
					hEnd++
				}
				if hEnd > hStart && isAlpha(content[hStart]) {
					// 合法 heredoc：记住标识符名称
					heredocIdent = string(content[hStart:hEnd])
					_ = indented // indented 模式只影响缩进，不影响结束标识匹配
					i = hEnd
					// 跳到行尾（heredoc 内容从下一行开始）
					for i < len(content) && content[i] != '\n' {
						i++
					}
					continue
				}
			}
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return string(content[startByte : i+1])
			}
		}
		i++
	}

	return ""
}

func isAlpha(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || b == '_'
}

func isIdentChar(b byte) bool {
	return isAlpha(b) || (b >= '0' && b <= '9') || b == '-'
}
