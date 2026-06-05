package services

import (
	"errors"
	"path/filepath"
	"sort"
	"strings"
)

// NormalizeManifestSubpath 归一化 workspace 的 manifest 执行子目录。
// 空 => "" (manifest 根)。禁绝对路径、. / .. 段、超长。与后端 manifest 文件路径规则一致。
// 从 controllers 包搬到此处,供 deployment install 与 workspace CRUD 共用同一套规则。
func NormalizeManifestSubpath(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	s = strings.ReplaceAll(s, "\\", "/")
	s = strings.Trim(s, "/")
	if s == "" {
		return "", nil
	}
	if len(s) > 512 {
		return "", errors.New("路径过长 (>512)")
	}
	for _, seg := range strings.Split(s, "/") {
		if seg == "." || seg == ".." {
			return "", errors.New("不允许 . 或 .. 路径段")
		}
		if seg == "" {
			return "", errors.New("路径段不能为空")
		}
	}
	return s, nil
}

// ListWorkdirs 返回一组 manifest 文件中所有"直接含 .tf 文件"的目录前缀(去重升序)。
//
// 语义与执行/解析一致:terraform 在 cd subpath 后只读该目录顶层 .tf(不递归),
// 所以每个 .tf 文件的所属目录即一个可用 workdir。根目录用 "" 表示(始终包含,
// 即使根下没有 .tf —— 让前端默认 "/" 可选;调用方可据 subpathExistsInVersion 再校验)。
func ListWorkdirs(scopeFiles map[string][]byte) []string {
	set := map[string]struct{}{"": {}} // 根始终可选
	for path := range scopeFiles {
		if filepath.Ext(path) != ".tf" {
			continue
		}
		dir := filepath.ToSlash(filepath.Dir(filepath.Clean(path)))
		if dir == "." {
			dir = ""
		}
		set[dir] = struct{}{}
	}
	out := make([]string, 0, len(set))
	for d := range set {
		out = append(out, d)
	}
	sort.Strings(out)
	return out
}
