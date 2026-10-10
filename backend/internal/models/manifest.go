package models

import (
	"encoding/json"
	"time"
)

// Manifest 可视化编排模板（Organization 级别）
type Manifest struct {
	ID             string `json:"id" gorm:"primaryKey;size:36"`              // 格式: mf-{ulid}
	OrganizationID int    `json:"organization_id" gorm:"not null;index"`     // 所属组织
	Name           string `json:"name" gorm:"size:255;not null"`             // 名称
	Description    string `json:"description" gorm:"type:text"`              // 描述
	Status         string `json:"status" gorm:"size:20;default:draft;index"` // draft, published, archived
	// 来源(创建时确定,不可修改;UpdateManifest 拒绝变更):native | git
	SourceType string `json:"source_type" gorm:"size:16;not null;default:native"`
	// git 来源字段(source_type=git 时 repo 必填;native 时全部为 NULL,DB CHECK 保证)。
	// git_repo_url = <GITHUB_URL>/<owner>/<repo>(无凭证);git_subpath = bundle 根目录
	// (空 = 仓库根);github_installation_id 必须是本组织已登记的 GitHub App installation。
	GitRepoURL           *string `json:"git_repo_url,omitempty" gorm:"column:git_repo_url;size:1024"`
	GitSubpath           *string `json:"git_subpath,omitempty" gorm:"column:git_subpath;size:512"`
	GitHubInstallationID *int64  `json:"github_installation_id,omitempty" gorm:"column:github_installation_id"`
	// 经验签 webhook 报告的仓库最新 push("有新 commit 可发布"),只提示,从不自动发布。
	GitLatestSHA *string    `json:"git_latest_sha,omitempty" gorm:"column:git_latest_sha;size:64"`
	GitLatestRef *string    `json:"git_latest_ref,omitempty" gorm:"column:git_latest_ref;size:255"`
	GitLatestAt  *time.Time `json:"git_latest_at,omitempty" gorm:"column:git_latest_at"`
	CreatedBy    string     `json:"created_by" gorm:"size:20;not null"` // 创建者
	CreatedAt    time.Time  `json:"created_at" gorm:"autoCreateTime"`   // 创建时间
	UpdatedAt    time.Time  `json:"updated_at" gorm:"autoUpdateTime"`   // 更新时间

	// 关联
	Versions    []ManifestVersion    `json:"versions,omitempty" gorm:"foreignKey:ManifestID"`
	Deployments []ManifestDeployment `json:"deployments,omitempty" gorm:"foreignKey:ManifestID"`

	// 非数据库字段
	LatestVersion   *ManifestVersion `json:"latest_version,omitempty" gorm:"-"`
	DeploymentCount int              `json:"deployment_count,omitempty" gorm:"-"`
	CreatedByName   string           `json:"created_by_name,omitempty" gorm:"-"`
	// 调用者能力标记(仅 list/get 填充;其他响应为 nil 不输出):
	//   can_write  = 调用者持有 MANIFESTS WRITE(组织级)
	//   can_deploy = 调用者在至少一个可见 workspace 上持有 WORKSPACE_RESOURCES WRITE
	//                (与 GET /workspaces?capability=WORKSPACE_RESOURCES:WRITE 同一判定;
	//                 MANIFESTS READ/WRITE 永不推出 can_deploy)
	CanWrite  *bool `json:"can_write,omitempty" gorm:"-"`
	CanDeploy *bool `json:"can_deploy,omitempty" gorm:"-"`
	//   can_admin  = 调用者持有 MANIFESTS ADMIN(删除 / 归档)
	CanAdmin *bool `json:"can_admin,omitempty" gorm:"-"`
}

func (Manifest) TableName() string {
	return "manifests"
}

// ManifestVersion Manifest 版本
//
// 新模型下版本只是 manifest_files 快照的元信息行,文件内容存 manifest_files
// (version_id = 本行 id)。画布相关旧字段 (canvas_data/nodes/edges/hcl_content/is_draft)
// 已在 PR4 移除。
type ManifestVersion struct {
	ID         string          `json:"id" gorm:"primaryKey;size:36"`              // 格式: mfv-{ulid}
	ManifestID string          `json:"manifest_id" gorm:"size:36;not null;index"` // 所属 Manifest
	Version    string          `json:"version" gorm:"size:50;not null"`           // 版本号，如 v1.0.0
	Variables  json.RawMessage `json:"variables" gorm:"type:jsonb"`               // 该版本声明的 Terraform input variables 元信息(.tf 静态解析)
	Changelog  string          `json:"changelog" gorm:"type:text"`                // 发布说明
	// 不可变 bundle 标识(manifestbundle.Hash,发布时写入;迁移 04 校验存量后改写为 v2)。
	// NULL = 该版本没有合法 bundle(原因见 BundleInvalidReason:"rule @ file" 或粘滞的
	// hash_mismatch),install / upgrade / 预览拒绝,需重新发布。读接口只返回存储值,不重算。
	BundleHash          *string `json:"bundle_hash" gorm:"column:bundle_hash;size:64"`
	BundleInvalidReason *string `json:"bundle_invalid_reason" gorm:"column:bundle_invalid_reason;type:text"`
	// git 来源:bundle 所钉的 commit SHA(native 为 NULL)。
	SourceRef *string   `json:"source_ref,omitempty" gorm:"column:source_ref;size:64"`
	CreatedBy string    `json:"created_by" gorm:"size:20;not null"` // 创建者
	CreatedAt time.Time `json:"created_at" gorm:"autoCreateTime"`   // 创建时间

	// 非数据库字段
	CreatedByName string `json:"created_by_name,omitempty" gorm:"-"`
}

func (ManifestVersion) TableName() string {
	return "manifest_versions"
}

// ManifestDeployment Manifest 部署记录
type ManifestDeployment struct {
	ID          string `json:"id" gorm:"primaryKey;size:36"`                        // 格式: mfd-{ulid}
	ManifestID  string `json:"manifest_id" gorm:"size:36;not null;index"`           // 所属 Manifest
	VersionID   string `json:"version_id" gorm:"size:36;not null"`                  // 部署的版本
	WorkspaceID string `json:"workspace_id" gorm:"type:varchar(50);not null;index"` // 目标 Workspace 语义化ID(对齐全平台)
	// 应急变量覆盖(扁平 key->string,优先级最高)。不直接输出:所有响应经 services.RedactOverrides
	// 输出 overrides: [{key, sensitive, has_value, value?}]。
	VariableOverrides json.RawMessage `json:"-" gorm:"type:jsonb"`
	Status            string          `json:"status" gorm:"size:20;default:active;index"` // active, uninstalled
	LastTaskID        *int            `json:"last_task_id" gorm:""`                       // 最后一次部署的任务 ID
	DeployedBy        string          `json:"deployed_by" gorm:"size:20;not null"`        // 部署者
	DeployedAt        *time.Time      `json:"deployed_at" gorm:""`                        // 部署时间
	// 审批绑定的双哈希(apply 前校验,任一不一致即拒绝);审批步骤接入前恒为 NULL,不在 API 输出
	ApprovedBundleHash *string `json:"-" gorm:"column:approved_bundle_hash;size:64"`
	ApprovedPlanHash   *string `json:"-" gorm:"column:approved_plan_hash;size:64"`
	// SensitiveKeys 覆盖值为敏感的 key(jsonb 字符串数组)。NULL = 尚未计算(启动回填前的存量行),
	// API 视为全部敏感;不在 API 输出。
	SensitiveKeys json.RawMessage `json:"-" gorm:"column:sensitive_keys;type:jsonb"`
	CreatedAt     time.Time       `json:"created_at" gorm:"autoCreateTime"` // 创建时间
	UpdatedAt     time.Time       `json:"updated_at" gorm:"autoUpdateTime"` // 更新时间

	// 关联
	Version   *ManifestVersion             `json:"version,omitempty" gorm:"foreignKey:VersionID"`
	Resources []ManifestDeploymentResource `json:"resources,omitempty" gorm:"foreignKey:DeploymentID"`

	// 非数据库字段
	WorkspaceName       string `json:"workspace_name,omitempty" gorm:"-"`
	WorkspaceSemanticID string `json:"workspace_semantic_id,omitempty" gorm:"-"` // ws-xxx 格式
	DeployedByName      string `json:"deployed_by_name,omitempty" gorm:"-"`
	VersionName         string `json:"version_name,omitempty" gorm:"-"`
}

func (ManifestDeployment) TableName() string {
	return "manifest_deployments"
}

// ManifestDeploymentResource 部署资源关联
type ManifestDeploymentResource struct {
	ID           string    `json:"id" gorm:"primaryKey;size:36"`                // 格式: mdr-{ulid}
	DeploymentID string    `json:"deployment_id" gorm:"size:36;not null;index"` // 所属部署
	NodeID       string    `json:"node_id" gorm:"size:50;not null"`             // Manifest 中的节点 ID
	ResourceID   string    `json:"resource_id" gorm:"size:255;not null;index"`  // workspace_resources.resource_id (语义 ID)
	ConfigHash   string    `json:"config_hash" gorm:"size:64"`                  // 部署时的配置 hash，用于漂移检测
	CreatedAt    time.Time `json:"created_at" gorm:"autoCreateTime"`            // 创建时间
}

func (ManifestDeploymentResource) TableName() string {
	return "manifest_deployment_resources"
}

// ========== 请求/响应结构 ==========

// CreateManifestRequest 创建 Manifest 请求
type CreateManifestRequest struct {
	Name        string `json:"name" binding:"required,max=255"`
	Description string `json:"description" binding:"max=1024"`
	// native(默认,平台内编辑)| git(GitHub App 只读,发布 = 选 commit)。创建后不可变。
	SourceType string `json:"source_type,omitempty"`
	// source_type=git 时必填其一(推荐 git_repo):仓库 full name "<owner>/<repo>",
	// 主机始终取平台配置 GITHUB_URL
	GitRepo string `json:"git_repo,omitempty" binding:"max=140"`
	// 兼容旧客户端:<GITHUB_URL>/<owner>/<repo>(必须是配置的主机)
	GitRepoURL string `json:"git_repo_url,omitempty" binding:"max=1024"`
	// source_type=git 时可选:仓库内作为 bundle 根的目录
	GitSubpath string `json:"git_subpath,omitempty" binding:"max=512"`
	// source_type=git 时必填:本组织已登记的 GitHub App installation(账户须是仓库 owner)
	GitHubInstallationID int64 `json:"github_installation_id,omitempty"`
}

// UpdateManifestRequest 更新 Manifest 请求
type UpdateManifestRequest struct {
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty" binding:"max=1024"`
	Status      string `json:"status,omitempty"` // draft, published, archived
	// source_type 创建后不可变:提供且与当前值不同 => 400
	SourceType string `json:"source_type,omitempty"`
	// git 源字段创建后不可变:提供且(规范化后)与当前值不同 => 409 git_source_immutable;
	// 原样回显允许。git_subpath 无效 => 400 git_subpath_invalid。
	GitRepoURL           *string `json:"git_repo_url,omitempty"`
	GitRepo              *string `json:"git_repo,omitempty"`
	GitSubpath           *string `json:"git_subpath,omitempty"`
	GitHubInstallationID *int64  `json:"github_installation_id,omitempty"`
}

// PublishManifestVersionRequest 发布版本请求
type PublishManifestVersionRequest struct {
	Version string `json:"version" binding:"required,max=50"` // 如 v1.0.0
}

// CreateManifestDeploymentRequest 创建部署请求
type CreateManifestDeploymentRequest struct {
	VersionID         string          `json:"version_id" binding:"required"`
	WorkspaceID       string          `json:"workspace_id" binding:"required"` // 语义化ID ws-xxx
	VariableOverrides json.RawMessage `json:"variable_overrides"`
	AutoApply         bool            `json:"auto_apply"` // 是否自动 Apply
	PlanOnly          bool            `json:"plan_only"`  // 仅 Plan
}

// UpdateManifestDeploymentRequest 更新部署请求
type UpdateManifestDeploymentRequest struct {
	VersionID         string          `json:"version_id,omitempty"`
	VariableOverrides json.RawMessage `json:"variable_overrides,omitempty"`
	AutoApply         bool            `json:"auto_apply"`
	PlanOnly          bool            `json:"plan_only"`
}

// ManifestCapabilities 调用者能力(与列表项的 can_* 同一判定)
//   - can_read / can_write / can_admin: 路由 MANIFESTS 检查得到的有效等级 >= READ / WRITE / ADMIN
//   - can_deploy: 至少一个可读 workspace 上有 WORKSPACE_RESOURCES WRITE(与 ?capability= 同一判定)
type ManifestCapabilities struct {
	CanRead   bool `json:"can_read"`
	CanWrite  bool `json:"can_write"`
	CanAdmin  bool `json:"can_admin"`
	CanDeploy bool `json:"can_deploy"`
}

// ManifestListResponse 列表响应
type ManifestListResponse struct {
	Items []Manifest `json:"items"`
	// 调用者在本组织 manifest 目录上的能力(列表为空时也返回,前端据此决定"新建"/"部署"入口)
	Capabilities *ManifestCapabilities `json:"capabilities,omitempty"`
	Total        int64                 `json:"total"`
	Page         int                   `json:"page"`
	PageSize     int                   `json:"page_size"`
	TotalPages   int                   `json:"total_pages"`
}

// ManifestVersionListResponse 版本列表响应
type ManifestVersionListResponse struct {
	Items      []ManifestVersion `json:"items"`
	Total      int64             `json:"total"`
	Page       int               `json:"page"`
	PageSize   int               `json:"page_size"`
	TotalPages int               `json:"total_pages"`
}

// ManifestDeploymentListResponse 部署列表响应
type ManifestDeploymentListResponse struct {
	Items      []ManifestDeployment `json:"items"`
	Total      int64                `json:"total"`
	Page       int                  `json:"page"`
	PageSize   int                  `json:"page_size"`
	TotalPages int                  `json:"total_pages"`
}

// ========== 常量定义 ==========

const (
	// Manifest 状态
	ManifestStatusDraft     = "draft"
	ManifestStatusPublished = "published"
	ManifestStatusArchived  = "archived"

	// Manifest 来源(manifests.source_type)
	ManifestSourceNative = "native"
	ManifestSourceGit    = "git"

	// 部署状态(新设计)
	DeploymentStatusActive      = "active"
	DeploymentStatusUninstalled = "uninstalled"

	// 部署状态(旧画布,兼容期保留;新代码不再写入)
	DeploymentStatusPending   = "pending"
	DeploymentStatusDeploying = "deploying"
	DeploymentStatusDeployed  = "deployed"
	DeploymentStatusFailed    = "failed"
	DeploymentStatusArchived  = "archived" // 已废弃

	// 节点类型
	NodeTypeModule   = "module"
	NodeTypeVariable = "variable"

	// 连接类型
	EdgeTypeDependency      = "dependency"
	EdgeTypeVariableBinding = "variable_binding"

	// 端口类型
	PortTypeInput  = "input"
	PortTypeOutput = "output"

	// 关联状态
	LinkStatusLinked   = "linked"
	LinkStatusUnlinked = "unlinked"
	LinkStatusMismatch = "mismatch"
)

// GitHubAppInstallation a GitHub App installation bound to an organization
// (github_app_installations). Bindings are created only by the App's setup
// callback, after user-to-server OAuth proved that the installing GitHub user
// administers the installation's account (VerifiedAt set); rows without
// VerifiedAt (manual registrations of earlier builds) are not usable. An
// installation belongs to at most one organization (unique installation_id).
// The GitHub user token used for the proof is never stored.
type GitHubAppInstallation struct {
	ID                   int64      `json:"id" gorm:"primaryKey"`
	OrganizationID       int        `json:"organization_id" gorm:"not null"`
	InstallationID       int64      `json:"installation_id" gorm:"not null"`
	AccountLogin         string     `json:"account_login" gorm:"size:255;not null"`
	AccountID            *int64     `json:"account_id,omitempty"`
	AccountType          *string    `json:"account_type,omitempty" gorm:"size:20"`
	VerifiedGitHubUserID *int64     `json:"verified_github_user_id,omitempty" gorm:"column:verified_github_user_id"`
	VerifiedGitHubLogin  *string    `json:"verified_github_login,omitempty" gorm:"column:verified_github_login;size:255"`
	VerifiedAt           *time.Time `json:"verified_at,omitempty"`
	CreatedBy            string     `json:"created_by" gorm:"size:20;not null"`
	CreatedAt            time.Time  `json:"created_at" gorm:"autoCreateTime"`
}

func (GitHubAppInstallation) TableName() string { return "github_app_installations" }

// GitHubAppSetupNonce a consumed setup-state nonce (single use).
type GitHubAppSetupNonce struct {
	Nonce          string    `gorm:"primaryKey;size:64"`
	OrganizationID int       `gorm:"not null"`
	UserID         string    `gorm:"size:20;not null"`
	ExpiresAt      time.Time `gorm:"not null"`
	ConsumedAt     time.Time `gorm:"not null"`
}

func (GitHubAppSetupNonce) TableName() string { return "github_app_setup_nonces" }

// GitHubWebhookDelivery an X-GitHub-Delivery id already processed (replay
// protection, kept 72h).
type GitHubWebhookDelivery struct {
	DeliveryID string    `gorm:"primaryKey;size:64"`
	Event      string    `gorm:"size:64;not null"`
	ReceivedAt time.Time `gorm:"not null"`
}

func (GitHubWebhookDelivery) TableName() string { return "github_webhook_deliveries" }
