package models

import (
	"encoding/json"
	"time"

	"gorm.io/datatypes"
)

// CMDBExternalSource 外部CMDB数据源配置
type CMDBExternalSource struct {
	ID          uint   `gorm:"column:id;primaryKey;autoIncrement" json:"-"`
	SourceID    string `gorm:"column:source_id;type:varchar(50);uniqueIndex;not null" json:"source_id"`
	Name        string `gorm:"column:name;type:varchar(100);not null" json:"name"`
	Description string `gorm:"column:description;type:text" json:"description,omitempty"`

	// API配置
	APIEndpoint string `gorm:"column:api_endpoint;type:varchar(500);not null" json:"api_endpoint"`
	HTTPMethod  string `gorm:"column:http_method;type:varchar(10);default:'GET'" json:"http_method"`
	RequestBody string `gorm:"column:request_body;type:text" json:"request_body,omitempty"`

	// 认证配置（Header）
	AuthHeaders datatypes.JSON `gorm:"column:auth_headers;type:jsonb" json:"auth_headers,omitempty"`

	// 数据映射配置
	ResponsePath string         `gorm:"column:response_path;type:varchar(200)" json:"response_path,omitempty"`
	FieldMapping datatypes.JSON `gorm:"column:field_mapping;type:jsonb;not null" json:"field_mapping"`

	// 主键配置
	PrimaryKeyField string `gorm:"column:primary_key_field;type:varchar(100);not null" json:"primary_key_field"`

	// 云环境配置
	CloudProvider string `gorm:"column:cloud_provider;type:varchar(50)" json:"cloud_provider,omitempty"`
	AccountID     string `gorm:"column:account_id;type:varchar(100)" json:"account_id,omitempty"`
	AccountName   string `gorm:"column:account_name;type:varchar(200)" json:"account_name,omitempty"`
	Region        string `gorm:"column:region;type:varchar(50)" json:"region,omitempty"`

	// 同步配置
	SyncIntervalMinutes int  `gorm:"column:sync_interval_minutes;default:60" json:"sync_interval_minutes"`
	IsEnabled           bool `gorm:"column:is_enabled;default:true" json:"is_enabled"`

	// 过滤配置
	ResourceTypeFilter string `gorm:"column:resource_type_filter;type:varchar(100)" json:"resource_type_filter,omitempty"`

	// 元数据
	OrganizationID  string     `gorm:"column:organization_id;type:varchar(50)" json:"organization_id,omitempty"`
	CreatedBy       string     `gorm:"column:created_by;type:varchar(50)" json:"created_by,omitempty"`
	UpdatedBy       string     `gorm:"column:updated_by;type:varchar(50)" json:"updated_by,omitempty"`
	CreatedAt       time.Time  `gorm:"column:created_at;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt       time.Time  `gorm:"column:updated_at;default:CURRENT_TIMESTAMP" json:"updated_at"`
	LastSyncAt      *time.Time `gorm:"column:last_sync_at" json:"last_sync_at,omitempty"`
	LastSyncStatus  string     `gorm:"column:last_sync_status;type:varchar(20)" json:"last_sync_status,omitempty"`
	LastSyncMessage string     `gorm:"column:last_sync_message;type:text" json:"last_sync_message,omitempty"`
	LastSyncCount   int        `gorm:"column:last_sync_count;default:0" json:"last_sync_count"`
}

// TableName 指定表名
func (CMDBExternalSource) TableName() string {
	return "cmdb_external_sources"
}

// AuthHeader 认证Header配置
type AuthHeader struct {
	Key      string `json:"key"`
	SecretID string `json:"secret_id,omitempty"` // 存储时使用
	Value    string `json:"value,omitempty"`     // 创建/更新时使用，不存储
	HasValue bool   `json:"has_value,omitempty"` // 响应时使用
}

// FieldMapping 字段映射配置
type FieldMapping struct {
	ResourceType      string `json:"resource_type,omitempty"`
	ResourceName      string `json:"resource_name,omitempty"`
	CloudResourceID   string `json:"cloud_resource_id,omitempty"`
	CloudResourceName string `json:"cloud_resource_name,omitempty"`
	CloudResourceARN  string `json:"cloud_resource_arn,omitempty"`
	Description       string `json:"description,omitempty"`
	Tags              string `json:"tags,omitempty"`
	Attributes        string `json:"attributes,omitempty"`
}

// GetAuthHeaders 获取认证Header列表
func (s *CMDBExternalSource) GetAuthHeaders() ([]AuthHeader, error) {
	if s.AuthHeaders == nil {
		return []AuthHeader{}, nil
	}
	var headers []AuthHeader
	if err := json.Unmarshal(s.AuthHeaders, &headers); err != nil {
		return nil, err
	}
	return headers, nil
}

// SetAuthHeaders 设置认证Header列表
func (s *CMDBExternalSource) SetAuthHeaders(headers []AuthHeader) error {
	data, err := json.Marshal(headers)
	if err != nil {
		return err
	}
	s.AuthHeaders = data
	return nil
}

// GetFieldMapping 获取字段映射配置
func (s *CMDBExternalSource) GetFieldMapping() (*FieldMapping, error) {
	if s.FieldMapping == nil {
		return &FieldMapping{}, nil
	}
	var mapping FieldMapping
	if err := json.Unmarshal(s.FieldMapping, &mapping); err != nil {
		return nil, err
	}
	return &mapping, nil
}

// SetFieldMapping 设置字段映射配置
func (s *CMDBExternalSource) SetFieldMapping(mapping *FieldMapping) error {
	data, err := json.Marshal(mapping)
	if err != nil {
		return err
	}
	s.FieldMapping = data
	return nil
}

// CMDBSyncLog 同步日志
type CMDBSyncLog struct {
	ID               uint       `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	SourceID         string     `gorm:"column:source_id;type:varchar(50);not null" json:"source_id"`
	StartedAt        time.Time  `gorm:"column:started_at;not null;default:CURRENT_TIMESTAMP" json:"started_at"`
	CompletedAt      *time.Time `gorm:"column:completed_at" json:"completed_at,omitempty"`
	Status           string     `gorm:"column:status;type:varchar(20);not null;default:'running'" json:"status"`
	ResourcesSynced  int        `gorm:"column:resources_synced;default:0" json:"resources_synced"`
	ResourcesAdded   int        `gorm:"column:resources_added;default:0" json:"resources_added"`
	ResourcesUpdated int        `gorm:"column:resources_updated;default:0" json:"resources_updated"`
	ResourcesDeleted int        `gorm:"column:resources_deleted;default:0" json:"resources_deleted"`
	ErrorMessage     string     `gorm:"column:error_message;type:text" json:"error_message,omitempty"`
}

// TableName 指定表名
func (CMDBSyncLog) TableName() string {
	return "cmdb_sync_logs"
}

// SyncStatus 同步状态常量
const (
	SyncStatusRunning = "running"
	SyncStatusSuccess = "success"
	SyncStatusFailed  = "failed"
)

// ===== Request/Response Models =====

// CreateExternalSourceRequest 创建外部数据源请求
type CreateExternalSourceRequest struct {
	Name                string            `json:"name" binding:"required,max=100"`
	Description         string            `json:"description"`
	APIEndpoint         string            `json:"api_endpoint" binding:"required,url"`
	HTTPMethod          string            `json:"http_method" binding:"omitempty,oneof=GET POST"`
	RequestBody         string            `json:"request_body"`
	AuthHeaders         []AuthHeaderInput `json:"auth_headers"`
	ResponsePath        string            `json:"response_path"`
	FieldMapping        map[string]string `json:"field_mapping" binding:"required"`
	PrimaryKeyField     string            `json:"primary_key_field" binding:"required"`
	CloudProvider       string            `json:"cloud_provider"`
	AccountID           string            `json:"account_id"`
	AccountName         string            `json:"account_name"`
	Region              string            `json:"region"`
	SyncIntervalMinutes int               `json:"sync_interval_minutes"`
	ResourceTypeFilter  string            `json:"resource_type_filter"`
}

// AuthHeaderInput 认证Header输入
type AuthHeaderInput struct {
	Key   string  `json:"key" binding:"required"`
	Value *string `json:"value"` // 指针类型，区分未提供和空字符串
}

// UpdateExternalSourceRequest 更新外部数据源请求
type UpdateExternalSourceRequest struct {
	Name                *string           `json:"name"`
	Description         *string           `json:"description"`
	APIEndpoint         *string           `json:"api_endpoint"`
	HTTPMethod          *string           `json:"http_method"`
	RequestBody         *string           `json:"request_body"`
	AuthHeaders         []AuthHeaderInput `json:"auth_headers"`
	ResponsePath        *string           `json:"response_path"`
	FieldMapping        map[string]string `json:"field_mapping"`
	PrimaryKeyField     *string           `json:"primary_key_field"`
	CloudProvider       *string           `json:"cloud_provider"`
	AccountID           *string           `json:"account_id"`
	AccountName         *string           `json:"account_name"`
	Region              *string           `json:"region"`
	SyncIntervalMinutes *int              `json:"sync_interval_minutes"`
	IsEnabled           *bool             `json:"is_enabled"`
	ResourceTypeFilter  *string           `json:"resource_type_filter"`
}

// ExternalSourceResponse 外部数据源响应
type ExternalSourceResponse struct {
	SourceID            string                 `json:"source_id"`
	Name                string                 `json:"name"`
	Description         string                 `json:"description,omitempty"`
	APIEndpoint         string                 `json:"api_endpoint"`
	HTTPMethod          string                 `json:"http_method"`
	RequestBody         string                 `json:"request_body,omitempty"`
	AuthHeaders         []AuthHeaderDisplay    `json:"auth_headers,omitempty"`
	ResponsePath        string                 `json:"response_path,omitempty"`
	FieldMapping        map[string]interface{} `json:"field_mapping"`
	PrimaryKeyField     string                 `json:"primary_key_field"`
	CloudProvider       string                 `json:"cloud_provider,omitempty"`
	AccountID           string                 `json:"account_id,omitempty"`
	AccountName         string                 `json:"account_name,omitempty"`
	Region              string                 `json:"region,omitempty"`
	SyncIntervalMinutes int                    `json:"sync_interval_minutes"`
	IsEnabled           bool                   `json:"is_enabled"`
	ResourceTypeFilter  string                 `json:"resource_type_filter,omitempty"`
	CreatedBy           string                 `json:"created_by,omitempty"`
	UpdatedBy           string                 `json:"updated_by,omitempty"`
	CreatedAt           time.Time              `json:"created_at"`
	UpdatedAt           time.Time              `json:"updated_at"`
	LastSyncAt          *time.Time             `json:"last_sync_at,omitempty"`
	LastSyncStatus      string                 `json:"last_sync_status,omitempty"`
	LastSyncMessage     string                 `json:"last_sync_message,omitempty"`
	LastSyncCount       int                    `json:"last_sync_count"`
}

// AuthHeaderDisplay 认证Header显示（不包含实际值）
type AuthHeaderDisplay struct {
	Key      string `json:"key"`
	HasValue bool   `json:"has_value"`
}

// ExternalSourceListResponse 外部数据源列表响应
type ExternalSourceListResponse struct {
	Sources []ExternalSourceResponse `json:"sources"`
	Total   int                      `json:"total"`
}

// TestConnectionResponse 测试连接响应
type TestConnectionResponse struct {
	Success     bool          `json:"success"`
	Message     string        `json:"message"`
	SampleCount int           `json:"sample_count,omitempty"`
	SampleData  []interface{} `json:"sample_data,omitempty"`
}

// SyncLogResponse 同步日志响应
type SyncLogResponse struct {
	ID               uint       `json:"id"`
	SourceID         string     `json:"source_id"`
	StartedAt        time.Time  `json:"started_at"`
	CompletedAt      *time.Time `json:"completed_at,omitempty"`
	Status           string     `json:"status"`
	ResourcesSynced  int        `json:"resources_synced"`
	ResourcesAdded   int        `json:"resources_added"`
	ResourcesUpdated int        `json:"resources_updated"`
	ResourcesDeleted int        `json:"resources_deleted"`
	ErrorMessage     string     `json:"error_message,omitempty"`
}

// SyncLogListResponse 同步日志列表响应
type SyncLogListResponse struct {
	Logs  []SyncLogResponse `json:"logs"`
	Total int               `json:"total"`
}

// ToResponse 转换为响应格式
func (s *CMDBExternalSource) ToResponse() (*ExternalSourceResponse, error) {
	// 解析auth_headers
	var authHeaders []AuthHeaderDisplay
	if s.AuthHeaders != nil {
		var headers []AuthHeader
		if err := json.Unmarshal(s.AuthHeaders, &headers); err == nil {
			for _, h := range headers {
				authHeaders = append(authHeaders, AuthHeaderDisplay{
					Key:      h.Key,
					HasValue: h.SecretID != "",
				})
			}
		}
	}

	// 解析field_mapping
	var fieldMapping map[string]interface{}
	if s.FieldMapping != nil {
		json.Unmarshal(s.FieldMapping, &fieldMapping)
	}

	return &ExternalSourceResponse{
		SourceID:            s.SourceID,
		Name:                s.Name,
		Description:         s.Description,
		APIEndpoint:         s.APIEndpoint,
		HTTPMethod:          s.HTTPMethod,
		RequestBody:         s.RequestBody,
		AuthHeaders:         authHeaders,
		ResponsePath:        s.ResponsePath,
		FieldMapping:        fieldMapping,
		PrimaryKeyField:     s.PrimaryKeyField,
		CloudProvider:       s.CloudProvider,
		AccountID:           s.AccountID,
		AccountName:         s.AccountName,
		Region:              s.Region,
		SyncIntervalMinutes: s.SyncIntervalMinutes,
		IsEnabled:           s.IsEnabled,
		ResourceTypeFilter:  s.ResourceTypeFilter,
		CreatedBy:           s.CreatedBy,
		UpdatedBy:           s.UpdatedBy,
		CreatedAt:           s.CreatedAt,
		UpdatedAt:           s.UpdatedAt,
		LastSyncAt:          s.LastSyncAt,
		LastSyncStatus:      s.LastSyncStatus,
		LastSyncMessage:     s.LastSyncMessage,
		LastSyncCount:       s.LastSyncCount,
	}, nil
}

// ToResponse 转换同步日志为响应格式
func (l *CMDBSyncLog) ToResponse() *SyncLogResponse {
	return &SyncLogResponse{
		ID:               l.ID,
		SourceID:         l.SourceID,
		StartedAt:        l.StartedAt,
		CompletedAt:      l.CompletedAt,
		Status:           l.Status,
		ResourcesSynced:  l.ResourcesSynced,
		ResourcesAdded:   l.ResourcesAdded,
		ResourcesUpdated: l.ResourcesUpdated,
		ResourcesDeleted: l.ResourcesDeleted,
		ErrorMessage:     l.ErrorMessage,
	}
}
