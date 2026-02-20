package models

type CreateModuleRequest struct {
	Name          string `json:"name" binding:"required"`
	Provider      string `json:"provider" binding:"required"`
	Source        string `json:"source"`
	Version       string `json:"version"`
	Description   string `json:"description"`
	RepositoryURL string `json:"repository_url"`
	Branch        string `json:"branch"`
}

type UpdateModuleRequest struct {
	Description string `json:"description"`
	Version     string `json:"version"`
	Branch      string `json:"branch"`
}

type CreateWorkspaceRequest struct {
	Name             string `json:"name" binding:"required"`
	Description      string `json:"description"`
	StateBackend     string `json:"state_backend" binding:"required"`
	TerraformVersion string `json:"terraform_version" binding:"required"`
	ExecutionMode    string `json:"execution_mode" binding:"required"`
}

type UpdateWorkspaceRequest struct {
	Description      string `json:"description"`
	TerraformVersion string `json:"terraform_version"`
	ExecutionMode    string `json:"execution_mode"`
}