package models

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// AssessmentLayer 评估层级
type AssessmentLayer string

const (
	AssessmentLayerSchema   AssessmentLayer = "schema"   // Schema 层：结构验证
	AssessmentLayerRule     AssessmentLayer = "rule"     // Rule 层：规则检查
	AssessmentLayerSemantic AssessmentLayer = "semantic" // Semantic 层：语义质量
)

// AssessmentVerdict 评估结论
type AssessmentVerdict string

const (
	AssessmentVerdictPass AssessmentVerdict = "pass" // 通过
	AssessmentVerdictWarn AssessmentVerdict = "warn" // 警告
	AssessmentVerdictFail AssessmentVerdict = "fail" // 失败
)

// AssessmentStatus 评估状态
type AssessmentStatus string

const (
	AssessmentStatusPending  AssessmentStatus = "pending"  // 待评估
	AssessmentStatusAssessed AssessmentStatus = "assessed" // 已评估
	AssessmentStatusPartial  AssessmentStatus = "partial"  // L1 完成，L2/L3 失败待补偿
)

// UserAction 用户操作
type UserAction string

const (
	UserActionAccepted UserAction = "accepted" // 接受
	UserActionModified UserAction = "modified" // 修改
	UserActionAborted  UserAction = "aborted"  // 中止
)

// TextArray 字符串数组类型，用于处理 PostgreSQL TEXT[] 和 SQLite JSON 数组
type TextArray []string

// Scan 实现 sql.Scanner 接口
func (a *TextArray) Scan(value interface{}) error {
	if value == nil {
		*a = nil
		return nil
	}

	switch v := value.(type) {
	case []byte:
		str := string(v)
		// PostgreSQL array format: {a,b,c}
		if strings.HasPrefix(str, "{") && strings.HasSuffix(str, "}") {
			str = strings.TrimPrefix(str, "{")
			str = strings.TrimSuffix(str, "}")
			if str == "" {
				*a = []string{}
				return nil
			}
			*a = strings.Split(str, ",")
			return nil
		}
		// JSON array format: ["a","b","c"] (for SQLite compatibility)
		if strings.HasPrefix(str, "[") {
			var arr []string
			if err := json.Unmarshal(v, &arr); err != nil {
				return fmt.Errorf("failed to unmarshal TextArray from JSON: %w", err)
			}
			*a = arr
			return nil
		}
		return errors.New("invalid TextArray format")
	case string:
		// PostgreSQL array format: {a,b,c}
		if strings.HasPrefix(v, "{") && strings.HasSuffix(v, "}") {
			str := strings.TrimPrefix(v, "{")
			str = strings.TrimSuffix(str, "}")
			if str == "" {
				*a = []string{}
				return nil
			}
			*a = strings.Split(str, ",")
			return nil
		}
		// JSON array format: ["a","b","c"] (for SQLite compatibility)
		if strings.HasPrefix(v, "[") {
			var arr []string
			if err := json.Unmarshal([]byte(v), &arr); err != nil {
				return fmt.Errorf("failed to unmarshal TextArray from JSON: %w", err)
			}
			*a = arr
			return nil
		}
		return errors.New("invalid TextArray format")
	default:
		return fmt.Errorf("unsupported type for TextArray: %T", value)
	}
}

// Value 实现 driver.Valuer 接口
func (a TextArray) Value() (driver.Value, error) {
	if a == nil {
		return nil, nil
	}
	// Use PostgreSQL array format: {a,b,c}
	return "{" + strings.Join(a, ",") + "}", nil
}

// SkillAssessmentResult Skill 评估结果模型
type SkillAssessmentResult struct {
	ID                     string             `gorm:"primaryKey;type:varchar(36)" json:"id"`
	UsageLogID             *string            `gorm:"type:varchar(36)" json:"usage_log_id,omitempty"`
	SkillName              string             `gorm:"type:varchar(128);not null" json:"skill_name"`
	SkillContentHash       string             `gorm:"type:varchar(64);not null" json:"skill_content_hash"`
	AssessedAt             time.Time          `gorm:"type:timestamptz;default:CURRENT_TIMESTAMP" json:"assessed_at"`
	AssessmentLayer        AssessmentLayer    `gorm:"type:varchar(16);not null" json:"assessment_layer"`
	Verdict                AssessmentVerdict  `gorm:"type:varchar(16);not null" json:"verdict"`
	Score                  int16              `gorm:"type:smallint;not null" json:"score"`
	AssessmentLatencyMs    *int               `gorm:"type:integer" json:"assessment_latency_ms,omitempty"`
	SchemaValid            *bool              `gorm:"type:boolean" json:"schema_valid,omitempty"`
	MissingFields          TextArray          `gorm:"type:text[]" json:"missing_fields,omitempty"`
	InvalidEnumFields      TextArray          `gorm:"type:text[]" json:"invalid_enum_fields,omitempty"`
	RuleViolations         *json.RawMessage   `gorm:"type:jsonb" json:"rule_violations,omitempty"`
	QualityIssues          *json.RawMessage   `gorm:"type:jsonb" json:"quality_issues,omitempty"`
	AssessmentConfidence   *string            `gorm:"type:varchar(16)" json:"assessment_confidence,omitempty"`
	AssessmentModel        *string            `gorm:"type:varchar(64)" json:"assessment_model,omitempty"`
	AssessmentRawOutput    *string            `gorm:"type:text" json:"assessment_raw_output,omitempty"`
	// --- Summary assessment fields ---
	SourceType             string            `gorm:"type:varchar(16);default:'skill'" json:"source_type"`
	ResourceID             *uint             `gorm:"type:integer" json:"resource_id,omitempty"`
	FormatViolations       TextArray         `gorm:"type:text[]" json:"format_violations,omitempty"`
	SecurityTagMisses      *json.RawMessage  `gorm:"type:jsonb" json:"security_tag_misses,omitempty"`
	HallucinationSuspects  TextArray         `gorm:"type:text[]" json:"hallucination_suspects,omitempty"`
}

// TableName 指定表名
func (SkillAssessmentResult) TableName() string {
	return "skill_assessment_results"
}

// SkillGoldenSet Golden Set 黄金测试集模型
type SkillGoldenSet struct {
	ID               string           `gorm:"primaryKey;type:varchar(36)" json:"id"`
	SkillName        string           `gorm:"type:varchar(128);not null" json:"skill_name"`
	AssessmentLayer  AssessmentLayer  `gorm:"type:varchar(16);not null" json:"assessment_layer"`
	InputSnapshot    json.RawMessage  `gorm:"type:jsonb;not null" json:"input_snapshot"`
	OutputSnapshot   json.RawMessage  `gorm:"type:jsonb;not null" json:"output_snapshot"`
	ExpectedVerdict  string           `gorm:"type:varchar(16);not null" json:"expected_verdict"`
	ExpectedScoreMin int              `gorm:"type:smallint;not null" json:"expected_score_min"`
	ExpectedScoreMax int              `gorm:"type:smallint;not null" json:"expected_score_max"`
	Annotations      *json.RawMessage `gorm:"type:jsonb" json:"annotations,omitempty"`
	CreatedBy        string           `gorm:"type:varchar(128)" json:"created_by,omitempty"`
	CreatedAt        time.Time        `json:"created_at"`
	IsActive         bool             `gorm:"default:true" json:"is_active"`
}

// TableName 指定表名
func (SkillGoldenSet) TableName() string { return "skill_golden_sets" }
