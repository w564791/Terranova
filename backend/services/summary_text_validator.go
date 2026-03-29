package services

import (
	"encoding/json"
	"regexp"
	"strings"
	"unicode/utf8"
)

// SummaryValidationResult L1 文本规则检测结果
type SummaryValidationResult struct {
	Verdict               string              `json:"verdict"` // pass | warn | fail
	Score                 int                 `json:"score"`   // 0-100
	FormatViolations      []string            `json:"format_violations,omitempty"`
	SecurityTagMisses     []map[string]string `json:"security_tag_misses,omitempty"`
	HallucinationSuspects []string            `json:"hallucination_suspects,omitempty"`
}

// SecurityTagRule 安全标注规则
type SecurityTagRule struct {
	Name        string
	AttrPattern *regexp.Regexp
	ExpectedTag string
	Severity    string // fail | warn
}

// resourceTypeCNMap 资源类型中文映射（用于首行格式检测）
var resourceTypeCNMap = map[string]string{
	"aws_security_group":          "安全组",
	"aws_instance":                "EC2实例",
	"aws_db_instance":             "RDS实例",
	"aws_s3_bucket":               "S3存储桶",
	"aws_vpc":                     "VPC",
	"aws_subnet":                  "子网",
	"aws_iam_role":                "IAM角色",
	"aws_iam_policy":              "IAM策略",
	"aws_lb":                      "负载均衡器",
	"aws_lambda_function":         "Lambda函数",
	"aws_ecs_service":             "ECS服务",
	"aws_ecs_cluster":             "ECS集群",
	"aws_elasticache_cluster":     "ElastiCache集群",
	"aws_cloudfront_distribution": "CloudFront分发",
}

// SummaryTextValidator L1 纯代码文本规则检测器
type SummaryTextValidator struct {
	securityRules    []SecurityTagRule
	markdownPatterns []*regexp.Regexp
	ipPattern        *regexp.Regexp
	instancePattern  *regexp.Regexp
	resourceIDPattern *regexp.Regexp
}

func NewSummaryTextValidator() *SummaryTextValidator {
	return &SummaryTextValidator{
		securityRules: []SecurityTagRule{
			{
				Name:        "公网暴露",
				AttrPattern: regexp.MustCompile(`(?:0\.0\.0\.0/0|::/0)`),
				ExpectedTag: "[公网暴露]",
				Severity:    "fail",
			},
			{
				Name:        "删除保护未启用",
				AttrPattern: regexp.MustCompile(`"deletion_protection"\s*:\s*(?:false|"false")`),
				ExpectedTag: "[删除保护未启用]",
				Severity:    "fail",
			},
			{
				Name:        "无备份",
				AttrPattern: regexp.MustCompile(`"backup_retention_period"\s*:\s*(?:0|"0")`),
				ExpectedTag: "[无备份]",
				Severity:    "warn",
			},
		},
		markdownPatterns: []*regexp.Regexp{
			regexp.MustCompile(`(?m)^#{1,6}\s`),
			regexp.MustCompile("(?m)```"),
			regexp.MustCompile(`(?m)^\s*[-*]\s`),
			regexp.MustCompile(`(?m)^\s*\d+\.\s`),
		},
		ipPattern:         regexp.MustCompile(`\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}(?:/\d{1,2})?\b`),
		instancePattern:   regexp.MustCompile(`\b[a-z]\d[a-z]?\.\w+\b`),
		resourceIDPattern: regexp.MustCompile(`\b(?:i-|sg-|vpc-|subnet-|vol-|eni-|rtb-|igw-|nat-|lb-)[a-z0-9]+\b`),
	}
}

// Validate 对单条摘要执行 L1 文本规则检测
func (v *SummaryTextValidator) Validate(summary string, resourceType string, attributes json.RawMessage) SummaryValidationResult {
	result := SummaryValidationResult{}
	var failCount, warnCount int
	attrsStr := string(attributes)

	// 1. Empty summary
	if strings.TrimSpace(summary) == "" {
		result.FormatViolations = append(result.FormatViolations, "empty_summary")
		result.Verdict = "fail"
		result.Score = 0
		return result
	}

	// 2. Over length (>200 chars)
	if utf8.RuneCountInString(summary) > 200 {
		result.FormatViolations = append(result.FormatViolations, "over_length")
		warnCount++
	}

	// 3. Markdown syntax
	for _, pattern := range v.markdownPatterns {
		if pattern.MatchString(summary) {
			result.FormatViolations = append(result.FormatViolations, "markdown_syntax")
			failCount++
			break
		}
	}

	// 4. First line format: should contain resource type CN name
	if cnName, ok := resourceTypeCNMap[resourceType]; ok {
		firstLine := strings.SplitN(summary, "\n", 2)[0]
		if !strings.Contains(firstLine, cnName) {
			result.FormatViolations = append(result.FormatViolations, "first_line_format")
			warnCount++
		}
	}

	// 5. Security tag checks
	for _, rule := range v.securityRules {
		if rule.AttrPattern.MatchString(attrsStr) && !strings.Contains(summary, rule.ExpectedTag) {
			miss := map[string]string{
				"rule":         rule.Name,
				"expected_tag": rule.ExpectedTag,
			}
			result.SecurityTagMisses = append(result.SecurityTagMisses, miss)
			if rule.Severity == "fail" {
				failCount++
			} else {
				warnCount++
			}
		}
	}

	// 6. Hallucination detection
	suspects := v.detectHallucinations(summary, attrsStr)
	result.HallucinationSuspects = suspects
	if len(suspects) > 0 {
		warnCount++
	}

	// Scoring
	if failCount > 0 {
		result.Verdict = "fail"
		score := 40 - failCount*10
		if score < 0 {
			score = 0
		}
		result.Score = score
	} else if warnCount > 0 {
		result.Verdict = "warn"
		score := 80 - warnCount*10
		if score < 60 {
			score = 60
		}
		result.Score = score
	} else {
		result.Verdict = "pass"
		result.Score = 100
	}

	return result
}

// detectHallucinations extracts verifiable values from summary and checks if they exist in attributes
func (v *SummaryTextValidator) detectHallucinations(summary, attrsStr string) []string {
	var suspects []string
	seen := make(map[string]bool)

	for _, ip := range v.ipPattern.FindAllString(summary, -1) {
		if !seen[ip] && !strings.Contains(attrsStr, ip) {
			suspects = append(suspects, ip)
			seen[ip] = true
		}
	}

	for _, inst := range v.instancePattern.FindAllString(summary, -1) {
		if !seen[inst] && !strings.Contains(attrsStr, inst) {
			suspects = append(suspects, inst)
			seen[inst] = true
		}
	}

	for _, rid := range v.resourceIDPattern.FindAllString(summary, -1) {
		if !seen[rid] && !strings.Contains(attrsStr, rid) {
			suspects = append(suspects, rid)
			seen[rid] = true
		}
	}

	return suspects
}
