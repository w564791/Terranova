package services

import (
	"encoding/json"
	"testing"
)

func TestSummaryValidator_EmptySummary(t *testing.T) {
	v := NewSummaryTextValidator()
	result := v.Validate("", "aws_instance", json.RawMessage(`{"id": "i-123"}`))
	if result.Verdict != "fail" {
		t.Errorf("expected fail for empty summary, got %s", result.Verdict)
	}
	if !containsStr(result.FormatViolations, "empty_summary") {
		t.Errorf("expected empty_summary violation, got %v", result.FormatViolations)
	}
}

func TestSummaryValidator_OverLength(t *testing.T) {
	v := NewSummaryTextValidator()
	long := make([]byte, 250)
	for i := range long {
		long[i] = 'a'
	}
	result := v.Validate(string(long), "aws_instance", json.RawMessage(`{}`))
	if result.Verdict != "warn" {
		t.Errorf("expected warn for over-length, got %s", result.Verdict)
	}
	if !containsStr(result.FormatViolations, "over_length") {
		t.Errorf("expected over_length violation, got %v", result.FormatViolations)
	}
}

func TestSummaryValidator_MarkdownDetection(t *testing.T) {
	v := NewSummaryTextValidator()
	tests := []struct {
		name    string
		summary string
	}{
		{"heading", "# EC2 实例 i-123\n配置信息"},
		{"code_block", "EC2实例\n```\nconfig\n```"},
		{"list_dash", "EC2实例\n- 配置项1"},
		{"list_star", "EC2实例\n* 配置项1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := v.Validate(tt.summary, "aws_instance", json.RawMessage(`{}`))
			if result.Verdict != "fail" {
				t.Errorf("expected fail for markdown '%s', got %s", tt.name, result.Verdict)
			}
			if !containsStr(result.FormatViolations, "markdown_syntax") {
				t.Errorf("expected markdown_syntax violation for '%s', got %v", tt.name, result.FormatViolations)
			}
		})
	}
}

func TestSummaryValidator_SecurityTag_PublicExposure(t *testing.T) {
	v := NewSummaryTextValidator()
	attrs := json.RawMessage(`{"ingress": [{"cidr_blocks": ["0.0.0.0/0"]}]}`)

	// Missing tag
	result := v.Validate("安全组 sg-123 入站规则允许 TCP 443", "aws_security_group", attrs)
	if result.Verdict != "fail" {
		t.Errorf("expected fail for missing [公网暴露], got %s", result.Verdict)
	}

	// Has tag
	result = v.Validate("安全组 sg-123 入站规则允许 TCP 443 [公网暴露]", "aws_security_group", attrs)
	hasMiss := false
	if result.SecurityTagMisses != nil {
		for _, m := range result.SecurityTagMisses {
			if m["rule"] == "公网暴露" {
				hasMiss = true
			}
		}
	}
	if hasMiss {
		t.Error("should not have 公网暴露 miss when tag is present")
	}
}

func TestSummaryValidator_SecurityTag_DeletionProtection(t *testing.T) {
	v := NewSummaryTextValidator()
	attrs := json.RawMessage(`{"deletion_protection": false}`)

	result := v.Validate("RDS实例 db-prod", "aws_db_instance", attrs)
	if result.Verdict != "fail" {
		t.Errorf("expected fail for missing [删除保护未启用], got %s", result.Verdict)
	}
}

func TestSummaryValidator_SecurityTag_NoBackup(t *testing.T) {
	v := NewSummaryTextValidator()
	attrs := json.RawMessage(`{"backup_retention_period": 0}`)

	result := v.Validate("RDS实例 db-test", "aws_db_instance", attrs)
	if result.Verdict != "warn" {
		t.Errorf("expected warn for missing [无备份], got %s", result.Verdict)
	}
}

func TestSummaryValidator_HallucinationDetection(t *testing.T) {
	v := NewSummaryTextValidator()
	attrs := json.RawMessage(`{"instance_type": "t3.micro", "id": "i-abc123"}`)
	summary := "EC2实例 i-abc123 实例类型 m5.large 位于 us-east-1"

	result := v.Validate(summary, "aws_instance", attrs)
	if !containsStr(result.HallucinationSuspects, "m5.large") {
		t.Errorf("expected m5.large as hallucination suspect, got %v", result.HallucinationSuspects)
	}
}

func TestSummaryValidator_AllPass(t *testing.T) {
	v := NewSummaryTextValidator()
	attrs := json.RawMessage(`{"id": "i-abc123", "instance_type": "t3.micro"}`)
	summary := "EC2实例 i-abc123 实例类型 t3.micro"

	result := v.Validate(summary, "aws_instance", attrs)
	if result.Verdict != "pass" {
		t.Errorf("expected pass, got %s (format=%v, security=%v, hallucination=%v)",
			result.Verdict, result.FormatViolations, result.SecurityTagMisses, result.HallucinationSuspects)
	}
	if result.Score != 100 {
		t.Errorf("expected score 100, got %d", result.Score)
	}
}

func containsStr(arr []string, target string) bool {
	for _, s := range arr {
		if s == target {
			return true
		}
	}
	return false
}
