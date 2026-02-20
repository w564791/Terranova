package models

import (
	"strings"
	"time"
)

// IaCEngineType IaC引擎类型（运行时推断，不存储在数据库）
type IaCEngineType string

const (
	IaCEngineTerraform IaCEngineType = "terraform"
	IaCEngineOpenTofu  IaCEngineType = "opentofu"
)

// TerraformVersion IaC引擎版本模型
// 支持 Terraform 和 OpenTofu，引擎类型从下载链接动态推断
type TerraformVersion struct {
	ID          int       `json:"id" db:"id"`
	Version     string    `json:"version" db:"version" binding:"required"`
	DownloadURL string    `json:"download_url" db:"download_url" binding:"required,url"`
	Checksum    string    `json:"checksum" db:"checksum" binding:"required,len=64"`
	Enabled     bool      `json:"enabled" db:"enabled"`
	Deprecated  bool      `json:"deprecated" db:"deprecated"`
	IsDefault   bool      `json:"is_default" db:"is_default"` // 是否为默认版本
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

// GetEngineType 根据下载链接动态获取引擎类型（不存储在数据库）
func (v *TerraformVersion) GetEngineType() IaCEngineType {
	return DetectEngineTypeFromURL(v.DownloadURL)
}

// DetectEngineTypeFromURL 根据下载链接自动检测引擎类型
func DetectEngineTypeFromURL(downloadURL string) IaCEngineType {
	url := strings.ToLower(downloadURL)

	// OpenTofu 特征检测
	if strings.Contains(url, "opentofu") ||
		strings.Contains(url, "tofu_") ||
		strings.Contains(url, "/tofu/") {
		return IaCEngineOpenTofu
	}

	// 默认为 Terraform
	return IaCEngineTerraform
}

// GetBinaryName 获取二进制文件名称
func (e IaCEngineType) GetBinaryName() string {
	switch e {
	case IaCEngineOpenTofu:
		return "tofu"
	default:
		return "terraform"
	}
}

// GetDisplayName 获取显示名称
func (e IaCEngineType) GetDisplayName() string {
	switch e {
	case IaCEngineOpenTofu:
		return "OpenTofu"
	default:
		return "Terraform"
	}
}

// CreateTerraformVersionRequest 创建Terraform版本请求
type CreateTerraformVersionRequest struct {
	Version     string `json:"version" binding:"required"`
	DownloadURL string `json:"download_url" binding:"required,url"`
	Checksum    string `json:"checksum" binding:"required,len=64"`
	Enabled     bool   `json:"enabled"`
	Deprecated  bool   `json:"deprecated"`
}

// UpdateTerraformVersionRequest 更新Terraform版本请求
type UpdateTerraformVersionRequest struct {
	DownloadURL *string `json:"download_url" binding:"omitempty,url"`
	Checksum    *string `json:"checksum" binding:"omitempty,len=64"`
	Enabled     *bool   `json:"enabled"`
	Deprecated  *bool   `json:"deprecated"`
}

// TerraformVersionListResponse Terraform版本列表响应
type TerraformVersionListResponse struct {
	Items []TerraformVersion `json:"items"`
	Total int                `json:"total"`
}
