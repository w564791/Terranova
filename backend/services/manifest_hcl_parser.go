package services

import (
	"path/filepath"
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
	parser := hclparse.NewParser()
	out := make(map[string]string)
	subpath = strings.TrimSuffix(subpath, "/")

	for path, content := range scopeFiles {
		if !shouldParseForResources(path, subpath) {
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
