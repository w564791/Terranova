package services

import (
	"path/filepath"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
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
					refs = append(refs, ManifestResourceRef{
						Kind: "resource",
						Type: block.Labels[0],
						Name: block.Labels[1],
						File: path,
					})
				}
			case "module":
				if len(block.Labels) >= 1 {
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
