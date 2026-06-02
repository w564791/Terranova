package services

import (
	"testing"
)

func TestParseManifestResources_DedupAndScope(t *testing.T) {
	files := map[string][]byte{
		"main.tf": []byte(`
resource "aws_vpc" "main" { cidr_block = "10.0.0.0/16" }
module "net" { source = "./net" }
`),
		// 同名 resource 在第二个文件再次声明(各自合法),应被去重成一行
		"extra.tf": []byte(`
resource "aws_vpc" "main" { cidr_block = "10.1.0.0/16" }
resource "aws_s3_bucket" "logs" { bucket = "logs" }
`),
		// 子目录文件不应被扫(terraform 不递归)
		"net/main.tf": []byte(`resource "aws_subnet" "a" {}`),
	}

	refs := ParseManifestResources(files, "")

	type key struct{ kind, typ, name string }
	count := map[key]int{}
	for _, r := range refs {
		count[key{r.Kind, r.Type, r.Name}]++
	}

	if count[key{"resource", "aws_vpc", "main"}] != 1 {
		t.Errorf("aws_vpc.main 应去重为 1 行, 实际 %d", count[key{"resource", "aws_vpc", "main"}])
	}
	if count[key{"resource", "aws_s3_bucket", "logs"}] != 1 {
		t.Errorf("aws_s3_bucket.logs 缺失")
	}
	if count[key{"module", "", "net"}] != 1 {
		t.Errorf("module.net 缺失")
	}
	if _, ok := count[key{"resource", "aws_subnet", "a"}]; ok {
		t.Errorf("子目录资源不应被解析")
	}
}

func TestParseManifestModuleSources(t *testing.T) {
	files := map[string][]byte{
		"main.tf": []byte(`
module "network" {
  source = "platform/aws-vpc"
  version = "5.5.1"
}
module "no_source_literal" {
  source = var.dynamic_source
}
resource "aws_vpc" "x" {}
`),
		"sub/main.tf": []byte(`module "ignored" { source = "platform/x" }`),
	}

	got := ParseManifestModuleSources(files, "")

	if got["network"] != "platform/aws-vpc" {
		t.Errorf("network source = %q, want platform/aws-vpc", got["network"])
	}
	if _, ok := got["no_source_literal"]; ok {
		t.Errorf("非字面量 source 不应被收录")
	}
	if _, ok := got["ignored"]; ok {
		t.Errorf("子目录 module 不应被解析")
	}
	if len(got) != 1 {
		t.Errorf("期望 1 个 module source, 实际 %d: %+v", len(got), got)
	}
}

func TestWorkspaceResourceIDConvention(t *testing.T) {
	res := ManifestResourceRef{Kind: "resource", Type: "aws_vpc", Name: "main"}
	if res.WorkspaceResourceID() != "aws_vpc.main" || res.WorkspaceResourceType() != "aws_vpc" {
		t.Errorf("resource ref: id=%q type=%q", res.WorkspaceResourceID(), res.WorkspaceResourceType())
	}
	mod := ManifestResourceRef{Kind: "module", Name: "network"}
	if mod.WorkspaceResourceID() != "module.network" || mod.WorkspaceResourceType() != "module" {
		t.Errorf("module ref: id=%q type=%q", mod.WorkspaceResourceID(), mod.WorkspaceResourceType())
	}
}

func TestIsTopLevelTFUnderSubpath(t *testing.T) {
	cases := []struct {
		path, subpath string
		want          bool
	}{
		{"main.tf", "", true},
		{"variables.tf", "", true},
		{"net/main.tf", "", false},        // 根目录扫描不递归子目录
		{"envs/prod/main.tf", "envs/prod", true},
		{"envs/prod/sub/x.tf", "envs/prod", false}, // subpath 下也不递归
		{"envs/dev/main.tf", "envs/prod", false},   // 不在 subpath 下
		{"main.tf.json", "", false},                // 只认 .tf
		{"notes.txt", "", false},
	}
	for _, c := range cases {
		if got := IsTopLevelTFUnderSubpath(c.path, c.subpath); got != c.want {
			t.Errorf("IsTopLevelTFUnderSubpath(%q,%q)=%v want %v", c.path, c.subpath, got, c.want)
		}
	}
}
