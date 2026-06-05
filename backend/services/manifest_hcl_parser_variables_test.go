package services

import (
	"testing"
)

func TestParseManifestVariables(t *testing.T) {
	files := map[string][]byte{
		"variables.tf": []byte(`
variable "region" {
  type        = string
  description = "AWS region"
  default     = "us-east-1"
}

variable "instance_count" {
  type = number
}

variable "db_password" {
  type      = string
  sensitive = true
}

variable "tags" {
  type    = map(string)
  default = { env = "prod" }
}
`),
		// 子目录文件不应被扫到(terraform 不递归)
		"modules/vpc/variables.tf": []byte(`
variable "should_be_ignored" {
  type = string
}
`),
		// 非 .tf 不扫
		"notes.txt": []byte(`variable "nope" { type = string }`),
	}

	metas := ParseManifestVariables(files)

	byName := make(map[string]ManifestVariableMeta, len(metas))
	for _, m := range metas {
		byName[m.Name] = m
	}

	if _, ok := byName["should_be_ignored"]; ok {
		t.Errorf("子目录变量不应被解析")
	}
	if _, ok := byName["nope"]; ok {
		t.Errorf("非 .tf 文件不应被解析")
	}
	if len(byName) != 4 {
		t.Fatalf("期望 4 个顶层变量, 实际 %d: %+v", len(byName), metas)
	}

	// region: 有 default => 非必填; type_raw/default_raw 取原文
	region := byName["region"]
	if region.Required {
		t.Errorf("region 有 default 应为非必填")
	}
	if region.TypeRaw != "string" {
		t.Errorf("region.TypeRaw = %q, want \"string\"", region.TypeRaw)
	}
	if region.DefaultRaw != `"us-east-1"` {
		t.Errorf("region.DefaultRaw = %q, want %q", region.DefaultRaw, `"us-east-1"`)
	}
	if region.Description != "AWS region" {
		t.Errorf("region.Description = %q", region.Description)
	}

	// instance_count: 无 default => 必填
	ic := byName["instance_count"]
	if !ic.Required {
		t.Errorf("instance_count 无 default 应为必填")
	}
	if ic.TypeRaw != "number" {
		t.Errorf("instance_count.TypeRaw = %q", ic.TypeRaw)
	}

	// db_password: sensitive
	pw := byName["db_password"]
	if !pw.Sensitive {
		t.Errorf("db_password 应为 sensitive")
	}
	if !pw.Required {
		t.Errorf("db_password 无 default 应为必填")
	}

	// tags: 复杂类型 / 复杂 default 原样存源码
	tags := byName["tags"]
	if tags.TypeRaw != "map(string)" {
		t.Errorf("tags.TypeRaw = %q, want \"map(string)\"", tags.TypeRaw)
	}
	if tags.DefaultRaw != `{ env = "prod" }` {
		t.Errorf("tags.DefaultRaw = %q", tags.DefaultRaw)
	}
	if tags.Required {
		t.Errorf("tags 有 default 应为非必填")
	}
}

func TestParseManifestVariables_BrokenHCLSkipped(t *testing.T) {
	files := map[string][]byte{
		"broken.tf": []byte(`variable "x" { type = `), // 未闭合
		"good.tf":   []byte(`variable "y" { type = string }`),
	}

	metas := ParseManifestVariables(files)

	// broken.tf 整文件 skip,good.tf 的 y 仍应解析出来
	found := false
	for _, m := range metas {
		if m.Name == "y" {
			found = true
		}
		if m.Name == "x" {
			t.Errorf("半成品 HCL 文件不应解析出变量")
		}
	}
	if !found {
		t.Errorf("good.tf 的变量 y 应被解析")
	}
}

func TestParseManifestVariables_Empty(t *testing.T) {
	// 没有 variable block 时返回空切片(非 nil 由调用方处理)
	metas := ParseManifestVariables(map[string][]byte{
		"main.tf": []byte(`resource "aws_vpc" "main" { cidr_block = "10.0.0.0/16" }`),
	})
	if len(metas) != 0 {
		t.Errorf("无 variable block 应返回空, 实际 %+v", metas)
	}
}
