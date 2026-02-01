package models

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"
)

// StringArray 自定义类型用于处理 JSONB 字符串数组
type StringArray []string

// Scan 实现 sql.Scanner 接口
func (s *StringArray) Scan(value interface{}) error {
	if value == nil {
		*s = []string{}
		return nil
	}

	bytes, ok := value.([]byte)
	if !ok {
		return errors.New("failed to scan StringArray")
	}

	return json.Unmarshal(bytes, s)
}

// Value 实现 driver.Valuer 接口
func (s StringArray) Value() (driver.Value, error) {
	if len(s) == 0 {
		return []byte("[]"), nil
	}
	return json.Marshal(s)
}

// CapabilityPrompts 自定义类型用于处理 JSONB 能力场景 prompt 映射
type CapabilityPrompts map[string]string

// Scan 实现 sql.Scanner 接口
func (c *CapabilityPrompts) Scan(value interface{}) error {
	if value == nil {
		*c = make(map[string]string)
		return nil
	}

	bytes, ok := value.([]byte)
	if !ok {
		return errors.New("failed to scan CapabilityPrompts")
	}

	return json.Unmarshal(bytes, c)
}

// Value 实现 driver.Valuer 接口
func (c CapabilityPrompts) Value() (driver.Value, error) {
	if len(c) == 0 {
		return []byte("{}"), nil
	}
	return json.Marshal(c)
}

// AIConfig AI 配置模型
type AIConfig struct {
	ID                  uint              `gorm:"primaryKey" json:"id"`
	ServiceType         string            `gorm:"type:varchar(50);not null;default:'bedrock'" json:"service_type"` // bedrock, openai, azure_openai, ollama 等
	AWSRegion           string            `gorm:"type:varchar(50)" json:"aws_region,omitempty"`                    // Bedrock 使用
	ModelID             string            `gorm:"type:varchar(200)" json:"model_id"`                               // 所有服务都需要
	BaseURL             string            `gorm:"type:varchar(500)" json:"base_url,omitempty"`                     // OpenAI Compatible API 基础 URL
	APIKey              string            `gorm:"type:text" json:"api_key,omitempty"`                              // OpenAI Compatible API 密钥（查询时不返回）
	CustomPrompt        string            `gorm:"type:text" json:"custom_prompt,omitempty"`
	Enabled             bool              `gorm:"default:false" json:"enabled"`
	RateLimitSeconds    int               `gorm:"default:10" json:"rate_limit_seconds"`              // 频率限制（秒）
	UseInferenceProfile bool              `gorm:"default:false" json:"use_inference_profile"`        // 是否使用 inference profile（仅 Bedrock）
	Capabilities        StringArray       `gorm:"type:jsonb;default:'[]'" json:"capabilities"`       // 支持的能力场景，["*"]表示默认配置，[]表示未配置
	CapabilityPrompts   CapabilityPrompts `gorm:"type:jsonb;default:'{}'" json:"capability_prompts"` // 每个能力场景的自定义 prompt
	Priority            int               `gorm:"default:0" json:"priority"`                         // 优先级，数值越大优先级越高
	// Skill 模式配置
	Mode             string           `gorm:"type:varchar(20);default:'prompt'" json:"mode"` // 模式：'prompt' 或 'skill'
	SkillComposition SkillComposition `gorm:"type:jsonb" json:"skill_composition,omitempty"` // Skill 组合配置（mode='skill'时使用）
	UseOptimized     bool             `gorm:"default:false" json:"use_optimized"`            // 是否使用优化版（并行执行 + AI 选择 Skills）
	// Vector 搜索配置（仅 embedding 能力使用）
	TopK                  int       `gorm:"default:50" json:"top_k"`                      // 向量搜索返回的最大结果数
	SimilarityThreshold   float64   `gorm:"default:0.3" json:"similarity_threshold"`      // 向量搜索相似度阈值（0-1）
	EmbeddingBatchEnabled bool      `gorm:"default:false" json:"embedding_batch_enabled"` // 是否启用批量 embedding
	EmbeddingBatchSize    int       `gorm:"default:10" json:"embedding_batch_size"`       // 批量大小（建议 10-50）
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

// TableName 指定表名
func (AIConfig) TableName() string {
	return "ai_configs"
}

// AIErrorAnalysis AI 错误分析结果模型
type AIErrorAnalysis struct {
	ID               uint      `gorm:"primaryKey" json:"id"`
	TaskID           uint      `gorm:"not null;uniqueIndex" json:"task_id"`
	UserID           string    `gorm:"type:varchar(20);not null;index" json:"user_id"`
	ErrorMessage     string    `gorm:"type:text;not null" json:"error_message"`
	ErrorType        string    `gorm:"type:varchar(100)" json:"error_type"`
	RootCause        string    `gorm:"type:text" json:"root_cause"`
	Solutions        string    `gorm:"type:jsonb" json:"solutions"` // JSON array
	Prevention       string    `gorm:"type:text" json:"prevention"`
	Severity         string    `gorm:"type:varchar(20)" json:"severity"`
	AnalysisDuration int       `json:"analysis_duration"` // 毫秒
	CreatedAt        time.Time `json:"created_at"`
}

// TableName 指定表名
func (AIErrorAnalysis) TableName() string {
	return "ai_error_analyses"
}

// AIAnalysisRateLimit AI 分析速率限制模型
type AIAnalysisRateLimit struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	UserID         string    `gorm:"type:varchar(20);not null;uniqueIndex" json:"user_id"`
	LastAnalysisAt time.Time `gorm:"not null" json:"last_analysis_at"`
}

// TableName 指定表名
func (AIAnalysisRateLimit) TableName() string {
	return "ai_analysis_rate_limits"
}

// BedrockModel Bedrock 模型信息
type BedrockModel struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Provider string `json:"provider"`
}
