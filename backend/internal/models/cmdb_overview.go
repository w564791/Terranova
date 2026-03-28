package models

import "time"

// CMDBOverview CMDB 观测面板数据
type CMDBOverview struct {
	Sources   CMDBOverviewSources   `json:"sources"`
	Resources CMDBOverviewResources `json:"resources"`
	Embedding CMDBOverviewEmbedding `json:"embedding"`
	Summary   CMDBOverviewSummary   `json:"summary"`
	Queue     CMDBOverviewQueue     `json:"queue"`
}

// CMDBSyncHistoryResponse 同步历史分页响应
type CMDBSyncHistoryResponse struct {
	Syncs []CMDBRecentSync `json:"syncs"`
	Total int64            `json:"total"`
	Page  int              `json:"page"`
	Size  int              `json:"size"`
}

// CMDBOverviewSources 数据源统计
type CMDBOverviewSources struct {
	WorkspaceCount        int64 `json:"workspace_count"`
	ExternalSourceCount   int64 `json:"external_source_count"`
	ExternalSourceHealthy int64 `json:"external_source_healthy"`
	ExternalSourceError   int64 `json:"external_source_error"`
}

// CMDBOverviewResources 资源统计
type CMDBOverviewResources struct {
	Total         int64              `json:"total"`
	FromWorkspace int64              `json:"from_workspace"`
	FromExternal  int64              `json:"from_external"`
	TypeTop10     []ResourceTypeStat `json:"type_top10"`
}

// CMDBOverviewEmbedding Embedding 覆盖率
type CMDBOverviewEmbedding struct {
	Total       int64   `json:"total"`
	Completed   int64   `json:"completed"`
	CoveragePct float64 `json:"coverage_pct"`
}

// CMDBOverviewSummary Summary 覆盖率
type CMDBOverviewSummary struct {
	Total       int64   `json:"total"`
	Completed   int64   `json:"completed"`
	CoveragePct float64 `json:"coverage_pct"`
}

// CMDBOverviewQueue 任务队列状态
type CMDBOverviewQueue struct {
	// Workspace embedding 任务队列 (embedding_tasks 表, EmbeddingWorker 消费)
	EmbeddingPending    int64 `json:"embedding_pending"`
	EmbeddingProcessing int64 `json:"embedding_processing"`
	EmbeddingFailed     int64 `json:"embedding_failed"`
	// 外部源 summary 任务队列 (cmdb_post_sync_jobs, job_type=summary)
	SummaryPending    int64 `json:"summary_pending"`
	SummaryProcessing int64 `json:"summary_processing"`
	SummaryFailed     int64 `json:"summary_failed"`
	// 外部源 embedding 任务队列 (cmdb_post_sync_jobs, job_type=embedding)
	ExtEmbeddingPending    int64 `json:"ext_embedding_pending"`
	ExtEmbeddingProcessing int64 `json:"ext_embedding_processing"`
	ExtEmbeddingFailed     int64 `json:"ext_embedding_failed"`
}

// CMDBRecentSync 最近同步记录
type CMDBRecentSync struct {
	SourceType       string     `json:"source_type" gorm:"column:source_type"`
	SourceID         string     `json:"source_id" gorm:"column:source_id"`
	SourceName       string     `json:"source_name" gorm:"column:source_name"`
	TriggeredBy      string     `json:"triggered_by" gorm:"column:triggered_by"`
	Status           string     `json:"status" gorm:"column:status"`
	StartedAt        time.Time  `json:"started_at" gorm:"column:started_at"`
	CompletedAt      *time.Time `json:"completed_at,omitempty" gorm:"column:completed_at"`
	ResourcesSynced  int        `json:"resources_synced" gorm:"column:resources_synced"`
	ResourcesAdded   int        `json:"resources_added" gorm:"column:resources_added"`
	ResourcesUpdated int        `json:"resources_updated" gorm:"column:resources_updated"`
	ResourcesDeleted int        `json:"resources_deleted" gorm:"column:resources_deleted"`
	ErrorMessage     string     `json:"error_message,omitempty" gorm:"column:error_message"`
}
