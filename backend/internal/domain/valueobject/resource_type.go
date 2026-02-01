package valueobject

import "fmt"

// ResourceType 资源类型
type ResourceType string

// 组织级资源
const (
	// ResourceTypeAppRegistration 应用注册
	ResourceTypeAppRegistration ResourceType = "APPLICATION_REGISTRATION"
	// ResourceTypeOrgSettings 组织设置
	ResourceTypeOrgSettings ResourceType = "ORGANIZATION"
	// ResourceTypeUserManagement 用户管理
	ResourceTypeUserManagement ResourceType = "USER_MANAGEMENT"
	// ResourceTypeAllProjects 所有项目
	ResourceTypeAllProjects ResourceType = "PROJECTS"
	// ResourceTypeAllWorkspaces 所有工作空间
	ResourceTypeAllWorkspaces ResourceType = "WORKSPACES"
	// ResourceTypeModules 模块管理
	ResourceTypeModules ResourceType = "MODULES"
	// ResourceTypeModuleDemos 模块Demo管理
	ResourceTypeModuleDemos ResourceType = "MODULE_DEMOS"
	// ResourceTypeSchemas Schema管理
	ResourceTypeSchemas ResourceType = "SCHEMAS"
	// ResourceTypeTaskLogs 任务日志
	ResourceTypeTaskLogs ResourceType = "TASK_LOGS"
	// ResourceTypeAgents Agent管理
	ResourceTypeAgents ResourceType = "AGENTS"
	// ResourceTypeAgentPools Agent Pool管理
	ResourceTypeAgentPools ResourceType = "AGENT_POOLS"
	// ResourceTypeIAMPermissions IAM权限管理
	ResourceTypeIAMPermissions ResourceType = "IAM_PERMISSIONS"
	// ResourceTypeIAMTeams IAM团队管理
	ResourceTypeIAMTeams ResourceType = "IAM_TEAMS"
	// ResourceTypeIAMOrganizations IAM组织管理
	ResourceTypeIAMOrganizations ResourceType = "IAM_ORGANIZATIONS"
	// ResourceTypeIAMProjects IAM项目管理
	ResourceTypeIAMProjects ResourceType = "IAM_PROJECTS"
	// ResourceTypeIAMApplications IAM应用管理
	ResourceTypeIAMApplications ResourceType = "IAM_APPLICATIONS"
	// ResourceTypeIAMAudit IAM审计日志
	ResourceTypeIAMAudit ResourceType = "IAM_AUDIT"
	// ResourceTypeIAMUsers IAM用户管理
	ResourceTypeIAMUsers ResourceType = "IAM_USERS"
	// ResourceTypeIAMRoles IAM角色管理
	ResourceTypeIAMRoles ResourceType = "IAM_ROLES"
	// ResourceTypeTerraformVersions Terraform版本管理
	ResourceTypeTerraformVersions ResourceType = "TERRAFORM_VERSIONS"
	// ResourceTypeAIConfigs AI配置管理
	ResourceTypeAIConfigs ResourceType = "AI_CONFIGS"
	// ResourceTypeAIAnalysis AI分析
	ResourceTypeAIAnalysis ResourceType = "AI_ANALYSIS"
)

// 项目级资源
const (
	// ResourceTypeProjectSettings 项目设置
	ResourceTypeProjectSettings ResourceType = "PROJECT_SETTINGS"
	// ResourceTypeProjectTeams 项目团队
	ResourceTypeProjectTeams ResourceType = "PROJECT_TEAM_MANAGEMENT"
	// ResourceTypeProjectWorkspaces 项目工作空间
	ResourceTypeProjectWorkspaces ResourceType = "PROJECT_WORKSPACES"
)

// 工作空间级资源
const (
	// ResourceTypeTaskData 任务数据
	ResourceTypeTaskData ResourceType = "TASK_DATA_ACCESS"
	// ResourceTypeWorkspaceExec 工作空间执行
	ResourceTypeWorkspaceExec ResourceType = "WORKSPACE_EXECUTION"
	// ResourceTypeWorkspaceState 状态管理
	ResourceTypeWorkspaceState ResourceType = "WORKSPACE_STATE"
	// ResourceTypeWorkspaceVars 变量管理
	ResourceTypeWorkspaceVars ResourceType = "WORKSPACE_VARIABLES"
	// ResourceTypeWorkspaceResources 资源管理
	ResourceTypeWorkspaceResources ResourceType = "WORKSPACE_RESOURCES"
	// ResourceTypeWorkspaceManagement 工作空间管理
	ResourceTypeWorkspaceManagement ResourceType = "WORKSPACE_MANAGEMENT"
)

// String 返回资源类型的字符串表示
func (r ResourceType) String() string {
	return string(r)
}

// IsValid 验证资源类型是否有效
func (r ResourceType) IsValid() bool {
	switch r {
	// 组织级
	case ResourceTypeAppRegistration, ResourceTypeOrgSettings,
		ResourceTypeUserManagement, ResourceTypeAllProjects, ResourceTypeAllWorkspaces, ResourceTypeModules,
		ResourceTypeModuleDemos, ResourceTypeSchemas, ResourceTypeTaskLogs,
		ResourceTypeAgents, ResourceTypeAgentPools,
		ResourceTypeIAMPermissions, ResourceTypeIAMTeams, ResourceTypeIAMOrganizations,
		ResourceTypeIAMProjects, ResourceTypeIAMApplications, ResourceTypeIAMAudit,
		ResourceTypeIAMUsers, ResourceTypeIAMRoles,
		ResourceTypeTerraformVersions, ResourceTypeAIConfigs, ResourceTypeAIAnalysis,
		// 项目级
		ResourceTypeProjectSettings, ResourceTypeProjectTeams,
		ResourceTypeProjectWorkspaces,
		// 工作空间级
		ResourceTypeTaskData, ResourceTypeWorkspaceExec,
		ResourceTypeWorkspaceState, ResourceTypeWorkspaceVars, ResourceTypeWorkspaceResources, ResourceTypeWorkspaceManagement:
		return true
	default:
		return false
	}
}

// GetScopeLevel 返回资源类型对应的作用域层级
func (r ResourceType) GetScopeLevel() ScopeType {
	switch r {
	// 组织级资源
	case ResourceTypeAppRegistration, ResourceTypeOrgSettings,
		ResourceTypeUserManagement, ResourceTypeAllProjects, ResourceTypeAllWorkspaces, ResourceTypeModules,
		ResourceTypeModuleDemos, ResourceTypeSchemas, ResourceTypeTaskLogs,
		ResourceTypeAgents, ResourceTypeAgentPools,
		ResourceTypeIAMPermissions, ResourceTypeIAMTeams, ResourceTypeIAMOrganizations,
		ResourceTypeIAMProjects, ResourceTypeIAMApplications, ResourceTypeIAMAudit,
		ResourceTypeIAMUsers, ResourceTypeIAMRoles,
		ResourceTypeTerraformVersions, ResourceTypeAIConfigs, ResourceTypeAIAnalysis:
		return ScopeTypeOrganization

	// 项目级资源
	case ResourceTypeProjectSettings, ResourceTypeProjectTeams,
		ResourceTypeProjectWorkspaces:
		return ScopeTypeProject

	// 工作空间级资源
	case ResourceTypeTaskData, ResourceTypeWorkspaceExec,
		ResourceTypeWorkspaceState, ResourceTypeWorkspaceVars, ResourceTypeWorkspaceResources, ResourceTypeWorkspaceManagement:
		return ScopeTypeWorkspace

	default:
		return ""
	}
}

// ParseResourceType 从字符串解析资源类型（忽略大小写）
func ParseResourceType(s string) (ResourceType, error) {
	// 转换为大写进行匹配
	upper := ResourceType(s)

	// 先尝试直接匹配大写版本
	if upper.IsValid() {
		return upper, nil
	}

	// 支持小写和混合大小写的映射
	lowerMap := map[string]ResourceType{
		"workspaces":           ResourceTypeAllWorkspaces,
		"WORKSPACES":           ResourceTypeAllWorkspaces,
		"modules":              ResourceTypeModules,
		"MODULES":              ResourceTypeModules,
		"organization":         ResourceTypeOrgSettings,
		"ORGANIZATION":         ResourceTypeOrgSettings,
		"workspace_management": ResourceTypeWorkspaceManagement,
		"WORKSPACE_MANAGEMENT": ResourceTypeWorkspaceManagement,
		"workspace_execution":  ResourceTypeWorkspaceExec,
		"WORKSPACE_EXECUTION":  ResourceTypeWorkspaceExec,
		"workspace_variables":  ResourceTypeWorkspaceVars,
		"WORKSPACE_VARIABLES":  ResourceTypeWorkspaceVars,
		"workspace_state":      ResourceTypeWorkspaceState,
		"WORKSPACE_STATE":      ResourceTypeWorkspaceState,
		"workspace_resources":  ResourceTypeWorkspaceResources,
		"WORKSPACE_RESOURCES":  ResourceTypeWorkspaceResources,
		"task_data_access":     ResourceTypeTaskData,
		"TASK_DATA_ACCESS":     ResourceTypeTaskData,
		"projects":             ResourceTypeAllProjects,
		"PROJECTS":             ResourceTypeAllProjects,
	}

	if rt, ok := lowerMap[s]; ok {
		return rt, nil
	}

	return "", fmt.Errorf("invalid resource type: %s", s)
}

// IsOrganizationLevel 判断是否为组织级资源
func (r ResourceType) IsOrganizationLevel() bool {
	return r.GetScopeLevel() == ScopeTypeOrganization
}

// IsProjectLevel 判断是否为项目级资源
func (r ResourceType) IsProjectLevel() bool {
	return r.GetScopeLevel() == ScopeTypeProject
}

// IsWorkspaceLevel 判断是否为工作空间级资源
func (r ResourceType) IsWorkspaceLevel() bool {
	return r.GetScopeLevel() == ScopeTypeWorkspace
}
