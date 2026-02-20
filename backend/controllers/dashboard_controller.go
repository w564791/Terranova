package controllers

import (
	"iac-platform/internal/models"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// DashboardController Dashboard控制器
type DashboardController struct {
	db *gorm.DB
}

// NewDashboardController 创建Dashboard控制器
func NewDashboardController(db *gorm.DB) *DashboardController {
	return &DashboardController{db: db}
}

// GetOverviewStats 获取概览统计
// @Summary 获取Dashboard概览统计
// @Description 获取平台的概览统计信息，包括项目数、工作空间数、任务统计等
// @Tags Dashboard
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{} "成功返回统计信息"
// @Router /api/v1/dashboard/overview [get]
// @Security Bearer
func (ctrl *DashboardController) GetOverviewStats(c *gin.Context) {
	// 1. Active projects (暂时等同于workspaces)
	var activeProjects int64
	ctrl.db.Model(&models.Workspace{}).Count(&activeProjects)

	// 2. Active workspaces
	var activeWorkspaces int64
	ctrl.db.Model(&models.Workspace{}).Count(&activeWorkspaces)

	// 3. Total applies (所有apply任务)
	var totalApplies int64
	ctrl.db.Model(&models.WorkspaceTask{}).
		Where("task_type IN (?, ?) AND status = ?",
			models.TaskTypeApply, models.TaskTypePlanAndApply, models.TaskStatusApplied).
		Count(&totalApplies)

	// 4. Applies this month
	startOfMonth := time.Now().AddDate(0, 0, -time.Now().Day()+1)
	var appliesThisMonth int64
	ctrl.db.Model(&models.WorkspaceTask{}).
		Where("task_type IN (?, ?) AND status = ? AND completed_at >= ?",
			models.TaskTypeApply, models.TaskTypePlanAndApply, models.TaskStatusApplied, startOfMonth).
		Count(&appliesThisMonth)

	// 5. Average applies per month (最近6个月)
	sixMonthsAgo := time.Now().AddDate(0, -6, 0)
	var appliesLast6Months int64
	ctrl.db.Model(&models.WorkspaceTask{}).
		Where("task_type IN (?, ?) AND status = ? AND completed_at >= ?",
			models.TaskTypeApply, models.TaskTypePlanAndApply, models.TaskStatusApplied, sixMonthsAgo).
		Count(&appliesLast6Months)
	averageAppliesPerMonth := appliesLast6Months / 6

	// 6. Billable Managed Resources (从最新的state versions统计)
	var totalResources int64
	ctrl.db.Model(&models.WorkspaceStateVersion{}).
		Select("COALESCE(SUM(resource_count), 0)").
		Where("id IN (SELECT MAX(id) FROM workspace_state_versions GROUP BY workspace_id)").
		Scan(&totalResources)

	// 7. Concurrent run limit reached (running tasks)
	var runningTasks int64
	ctrl.db.Model(&models.WorkspaceTask{}).
		Where("status = ?", models.TaskStatusRunning).
		Count(&runningTasks)

	// 8. Active agents
	var activeAgents int64
	ctrl.db.Model(&models.Agent{}).
		Where("status = ?", "online").
		Count(&activeAgents)

	var totalAgents int64
	ctrl.db.Model(&models.Agent{}).Count(&totalAgents)

	c.JSON(200, gin.H{
		"active_projects":            activeProjects,
		"active_workspaces":          activeWorkspaces,
		"total_applies":              totalApplies,
		"applies_this_month":         appliesThisMonth,
		"average_applies_per_month":  averageAppliesPerMonth,
		"billable_managed_resources": totalResources,
		"billable_limit":             500, // 固定限制
		"concurrent_run_limit":       runningTasks,
		"concurrent_limit":           1, // 固定限制
		"active_agents":              activeAgents,
		"total_agents":               totalAgents,
	})
}

// GetComplianceStats 获取合规统计（改为任务统计）
// @Summary 获取任务统计信息
// @Description 获取任务的成功、失败、待处理等统计信息
// @Tags Dashboard
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{} "成功返回任务统计"
// @Router /api/v1/dashboard/compliance [get]
// @Security Bearer
func (ctrl *DashboardController) GetComplianceStats(c *gin.Context) {
	// 1. 成功的任务数
	var successfulTasks int64
	ctrl.db.Model(&models.WorkspaceTask{}).
		Where("status IN (?, ?)", models.TaskStatusSuccess, models.TaskStatusApplied).
		Count(&successfulTasks)

	// 2. 失败的任务数
	var failedTasks int64
	ctrl.db.Model(&models.WorkspaceTask{}).
		Where("status = ?", models.TaskStatusFailed).
		Count(&failedTasks)

	// 3. 待处理的任务数
	var pendingTasks int64
	ctrl.db.Model(&models.WorkspaceTask{}).
	Where("status IN (?, ?, ?)", models.TaskStatusPending, models.TaskStatusWaiting, models.TaskStatusApplyPending).
	Count(&pendingTasks)

	// 4. 总任务数
	var totalTasks int64
	ctrl.db.Model(&models.WorkspaceTask{}).Count(&totalTasks)

	c.JSON(200, gin.H{
		"run_task_integrations": map[string]interface{}{
			"current": successfulTasks,
		},
		"workspace_run_tasks": map[string]interface{}{
			"current": failedTasks,
		},
		"policy_sets": map[string]interface{}{
			"current": pendingTasks,
		},
		"policies": map[string]interface{}{
			"current": totalTasks,
		},
	})
}
