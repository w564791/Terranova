package models

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"
)

// SkillLayer Skill 层级类型
type SkillLayer string

const (
	SkillLayerFoundation SkillLayer = "foundation" // 基础层：最通用的基础知识
	SkillLayerDomain     SkillLayer = "domain"     // 领域层：专业领域知识
	SkillLayerTask       SkillLayer = "task"       // 任务层：特定功能的工作流
)

// SkillSourceType Skill 来源类型
type SkillSourceType string

const (
	SkillSourceManual     SkillSourceType = "manual"      // 手动创建
	SkillSourceModuleAuto SkillSourceType = "module_auto" // Module 自动生成
	SkillSourceHybrid     SkillSourceType = "hybrid"      // 自动生成后手动修改
)

// SkillMetadata Skill 元数据
type SkillMetadata struct {
	Tags        []string `json:"tags,omitempty"`        // 标签（Domain Skill 用于被发现）
	DomainTags  []string `json:"domain_tags,omitempty"` // 需要的领域标签（Task Skill 用于发现 Domain Skills）
	Description string   `json:"description,omitempty"` // 描述
	Author      string   `json:"author,omitempty"`      // 作者
	UsageCount  int      `json:"usage_count,omitempty"` // 使用次数
	AvgRating   float64  `json:"avg_rating,omitempty"`  // 平均评分
}

// Scan 实现 sql.Scanner 接口
func (m *SkillMetadata) Scan(value interface{}) error {
	if value == nil {
		*m = SkillMetadata{}
		return nil
	}

	bytes, ok := value.([]byte)
	if !ok {
		return errors.New("failed to scan SkillMetadata")
	}

	return json.Unmarshal(bytes, m)
}

// Value 实现 driver.Valuer 接口
func (m SkillMetadata) Value() (driver.Value, error) {
	return json.Marshal(m)
}

// Skill AI Skill 知识单元模型
type Skill struct {
	ID             string          `gorm:"primaryKey;type:varchar(36)" json:"id"`
	Name           string          `gorm:"type:varchar(255);not null;uniqueIndex" json:"name"`
	DisplayName    string          `gorm:"type:varchar(255);not null" json:"display_name"`
	Description    string          `gorm:"type:varchar(500)" json:"description"` // 简短描述，用于 AI 智能选择
	Layer          SkillLayer      `gorm:"type:varchar(20);not null" json:"layer"`
	Content        string          `gorm:"type:text;not null" json:"content"`
	Version        string          `gorm:"type:varchar(50);default:'1.0.0'" json:"version"`
	IsActive       bool            `gorm:"default:true" json:"is_active"`
	Priority       int             `gorm:"default:0" json:"priority"`
	SourceType     SkillSourceType `gorm:"type:varchar(50);not null" json:"source_type"`
	SourceModuleID *uint           `gorm:"type:integer" json:"source_module_id,omitempty"`
	Metadata       SkillMetadata   `gorm:"type:jsonb;default:'{}'" json:"metadata"`
	CreatedBy      string          `gorm:"type:varchar(20)" json:"created_by,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`

	// 关联（非数据库字段）
	SourceModule *Module `gorm:"foreignKey:SourceModuleID" json:"source_module,omitempty"`
}

// TableName 指定表名
func (Skill) TableName() string {
	return "skills"
}

// SkillUsageLog Skill 使用日志模型
type SkillUsageLog struct {
	ID              string    `gorm:"primaryKey;type:varchar(36)" json:"id"`
	SkillIDs        []string  `gorm:"type:jsonb;not null;serializer:json" json:"skill_ids"`
	Capability      string    `gorm:"type:varchar(100);not null" json:"capability"`
	WorkspaceID     string    `gorm:"type:varchar(20)" json:"workspace_id,omitempty"`
	UserID          string    `gorm:"type:varchar(20);not null" json:"user_id"`
	ModuleID        *uint     `gorm:"type:integer" json:"module_id,omitempty"`
	ExecutionTimeMs int       `gorm:"type:integer" json:"execution_time_ms,omitempty"`
	UserFeedback    *int      `gorm:"type:integer" json:"user_feedback,omitempty"`
	AIModel         string    `gorm:"type:varchar(100)" json:"ai_model,omitempty"`
	ContextSummary  string    `gorm:"type:text" json:"context_summary,omitempty"`
	ResponseSummary string    `gorm:"type:text" json:"response_summary,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

// TableName 指定表名
func (SkillUsageLog) TableName() string {
	return "skill_usage_logs"
}

// ========== Skill 组合配置结构 ==========

// DomainSkillMode Domain Skill 加载模式
type DomainSkillMode string

const (
	DomainSkillModeFixed  DomainSkillMode = "fixed"  // 固定选择：只使用 domain_skills 中手动选择的
	DomainSkillModeAuto   DomainSkillMode = "auto"   // 自动发现：从 Task Skill 内容中解析依赖
	DomainSkillModeHybrid DomainSkillMode = "hybrid" // 混合模式：固定选择 + 自动发现补充
)

// SkillComposition Skill 组合配置
type SkillComposition struct {
	FoundationSkills    []string               `json:"foundation_skills"`      // Foundation 层 Skill 名称列表
	DomainSkills        []string               `json:"domain_skills"`          // Domain 层 Skill 名称列表（固定选择）
	TaskSkill           string                 `json:"task_skill"`             // Task 层 Skill 名称
	AutoLoadModuleSkill bool                   `json:"auto_load_module_skill"` // 是否自动加载 Module 生成的 Skill
	DomainSkillMode     DomainSkillMode        `json:"domain_skill_mode"`      // Domain Skill 加载模式：fixed/auto/hybrid
	ConditionalRules    []SkillConditionalRule `json:"conditional_rules"`      // 条件加载规则
}

// Scan 实现 sql.Scanner 接口
func (c *SkillComposition) Scan(value interface{}) error {
	if value == nil {
		*c = SkillComposition{}
		return nil
	}

	bytes, ok := value.([]byte)
	if !ok {
		return errors.New("failed to scan SkillComposition")
	}

	return json.Unmarshal(bytes, c)
}

// Value 实现 driver.Valuer 接口
func (c SkillComposition) Value() (driver.Value, error) {
	return json.Marshal(c)
}

// SkillConditionalRule 条件加载规则
type SkillConditionalRule struct {
	Condition string   `json:"condition"`  // 条件表达式，如 "use_cmdb == true"
	AddSkills []string `json:"add_skills"` // 满足条件时额外加载的 Skill 名称列表
}

// ========== 请求/响应结构 ==========

// CreateSkillRequest 创建 Skill 请求
type CreateSkillRequest struct {
	Name        string          `json:"name" binding:"required,max=255"`
	DisplayName string          `json:"display_name" binding:"required,max=255"`
	Description string          `json:"description,omitempty" binding:"max=500"` // 简短描述，用于 AI 智能选择
	Layer       SkillLayer      `json:"layer" binding:"required,oneof=foundation domain task"`
	Content     string          `json:"content" binding:"required"`
	Version     string          `json:"version,omitempty"`
	Priority    int             `json:"priority,omitempty"`
	SourceType  SkillSourceType `json:"source_type,omitempty"`
	Metadata    *SkillMetadata  `json:"metadata,omitempty"`
}

// UpdateSkillRequest 更新 Skill 请求
type UpdateSkillRequest struct {
	DisplayName *string        `json:"display_name,omitempty"`
	Description *string        `json:"description,omitempty"` // 简短描述，用于 AI 智能选择
	Content     *string        `json:"content,omitempty"`
	Version     *string        `json:"version,omitempty"`
	IsActive    *bool          `json:"is_active,omitempty"`
	Priority    *int           `json:"priority,omitempty"`
	Metadata    *SkillMetadata `json:"metadata,omitempty"`
}

// SkillListResponse Skill 列表响应
type SkillListResponse struct {
	Skills     []Skill `json:"skills"`
	Total      int64   `json:"total"`
	Page       int     `json:"page"`
	PageSize   int     `json:"page_size"`
	TotalPages int     `json:"total_pages"`
}

// SkillUsageStats Skill 使用统计
type SkillUsageStats struct {
	SkillID       string     `json:"skill_id"`
	SkillName     string     `json:"skill_name"`
	UsageCount    int64      `json:"usage_count"`
	AvgRating     float64    `json:"avg_rating"`
	AvgExecTimeMs float64    `json:"avg_exec_time_ms"`
	LastUsedAt    *time.Time `json:"last_used_at,omitempty"`
}

// ========== AI 配置扩展 ==========

// AIConfigMode AI 配置模式
type AIConfigMode string

const (
	AIConfigModePrompt AIConfigMode = "prompt" // 提示词模式（默认）
	AIConfigModeSkill  AIConfigMode = "skill"  // Skill 组合模式
)

// ========== 辅助方法 ==========

// IsFoundation 判断是否为 Foundation 层
func (s *Skill) IsFoundation() bool {
	return s.Layer == SkillLayerFoundation
}

// IsDomain 判断是否为 Domain 层
func (s *Skill) IsDomain() bool {
	return s.Layer == SkillLayerDomain
}

// IsTask 判断是否为 Task 层
func (s *Skill) IsTask() bool {
	return s.Layer == SkillLayerTask
}

// IsManual 判断是否为手动创建
func (s *Skill) IsManual() bool {
	return s.SourceType == SkillSourceManual
}

// IsModuleAuto 判断是否为 Module 自动生成
func (s *Skill) IsModuleAuto() bool {
	return s.SourceType == SkillSourceModuleAuto
}

// IsHybrid 判断是否为混合模式（自动生成后手动修改）
func (s *Skill) IsHybrid() bool {
	return s.SourceType == SkillSourceHybrid
}
