package middleware

import (
	"errors"
	"log"
	"net/http"
	"time"

	"iac-platform/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Agent task ownership (agent-facing task and workspace API).
//
// The caller is an agent (per-agent token: the agent ID comes from the
// token) or, for older agents, a pool (pool token, shared by every agent of
// the pool; see agent_auth.go). A task call is allowed only when
//   - agent token: the task's agent_id is the token's agent;
//   - pool token: the task is assigned to an agent of the calling pool that
//     does NOT have models.AgentCapabilityAgentTokenV1 (such an agent always
//     uses its token, so a pool token cannot act on its tasks);
//   - and the task is in a state in which its agent may use it (policy).
// The X-Agent-ID header of the previous scheme is not read.
//
// Ownership mismatch → 403 (the task-check middleware before this one already
// answers 403 for a task outside the pool's workspaces, so existence is not
// hidden at this layer; 403 keeps one code for "not yours"). A task in the
// wrong state → 409.

// AgentTaskPolicy the task states an endpoint accepts.
type AgentTaskPolicy int

const (
	// AgentTaskExecuting: status running (task data, plan task / plan data
	// fetch, plan uploads, log reads).
	AgentTaskExecuting AgentTaskPolicy = iota
	// AgentTaskReporting: running, or a state the executing agent itself
	// ends the task in, within AgentTaskReportGrace of completion (status
	// updates and their retries, the last log chunk, resource-change
	// derivation requests from older agents, the post-apply unlock).
	AgentTaskReporting
)

// AgentTaskReportGrace how long after completion the task's agent may still
// report (status retries, final logs, unlock).
var AgentTaskReportGrace = 15 * time.Minute

// agentReportStatuses the states an executing agent ends a task in.
var agentReportStatuses = []models.TaskStatus{
	models.TaskStatusApplyPending, models.TaskStatusPlannedAndFinished, models.TaskStatusSuccess,
	models.TaskStatusApplied, models.TaskStatusFailed, models.TaskStatusCancelled, "partial_success",
}

func isAgentReportStatus(s models.TaskStatus) bool {
	for _, r := range agentReportStatuses {
		if r == s {
			return true
		}
	}
	return false
}

type agentTaskRow struct {
	ID          uint
	AgentID     *string
	Status      models.TaskStatus
	CompletedAt *time.Time
	UpdatedAt   time.Time
}

func (t *agentTaskRow) allowedBy(policy AgentTaskPolicy, now time.Time) bool {
	if t.Status == models.TaskStatusRunning {
		return true
	}
	if policy != AgentTaskReporting || !isAgentReportStatus(t.Status) {
		return false
	}
	ended := t.UpdatedAt
	if t.CompletedAt != nil {
		ended = *t.CompletedAt
	}
	return now.Sub(ended) <= AgentTaskReportGrace
}

// AgentMayUseTask whether a task in this state may still be used by its
// agent under policy (also used by the C&C channel).
func AgentMayUseTask(policy AgentTaskPolicy, status models.TaskStatus, completedAt *time.Time, updatedAt, now time.Time) bool {
	t := agentTaskRow{Status: status, CompletedAt: completedAt, UpdatedAt: updatedAt}
	return t.allowedBy(policy, now)
}

// errAgentNotOwner the calling agent is not the task's agent.
var errAgentNotOwner = errors.New("task is not assigned to the calling agent")

// verifyTaskAgent checks that agentID (the task's agent) is the calling
// agent (agent token) or an older agent of the calling pool (pool token).
func verifyTaskAgent(db *gorm.DB, c *gin.Context, poolID, agentID string) error {
	if tokAgent := TokenAgentID(c); tokAgent != "" {
		if tokAgent != agentID {
			return errAgentNotOwner
		}
		return nil // agent row (pool, revocation) checked at authentication
	}
	var agent models.Agent
	err := db.WithContext(c.Request.Context()).Select("agent_id", "pool_id", "capabilities").
		Where("agent_id = ? AND pool_id = ?", agentID, poolID).Take(&agent).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return errAgentNotOwner
	}
	if err != nil {
		return err
	}
	if agent.HasCapability(models.AgentCapabilityAgentTokenV1) {
		return errAgentNotOwner // this agent authenticates with its own token
	}
	return nil
}

// RequireTaskAgent binds a task route (after PoolTokenAuthWithTaskCheck) to
// the task's agent and the policy's states.
func RequireTaskAgent(db *gorm.DB, policy AgentTaskPolicy) gin.HandlerFunc {
	return func(c *gin.Context) {
		poolID := c.GetString("pool_id")
		taskID, ok := c.Get("authorized_task_id")
		if poolID == "" || !ok {
			respondWithError(c, http.StatusInternalServerError, "task authorization context missing")
			return
		}
		var task agentTaskRow
		if err := db.WithContext(c.Request.Context()).Table("workspace_tasks").
			Select("id, agent_id, status, completed_at, updated_at").
			Where("id = ?", taskID).Take(&task).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				respondWithError(c, http.StatusNotFound, "Task not found")
				return
			}
			respondWithError(c, http.StatusInternalServerError, "Failed to query task")
			return
		}
		if task.AgentID == nil || *task.AgentID == "" {
			respondWithError(c, http.StatusForbidden, "task is not assigned to an agent")
			return
		}
		if err := verifyTaskAgent(db, c, poolID, *task.AgentID); err != nil {
			if errors.Is(err, errAgentNotOwner) {
				log.Printf("[AgentAuth] [WARN] pool %s caller %q denied task %d (assigned to %s)",
					poolID, TokenAgentID(c), task.ID, *task.AgentID)
				respondWithError(c, http.StatusForbidden, errAgentNotOwner.Error())
				return
			}
			respondWithError(c, http.StatusInternalServerError, "Failed to verify task agent")
			return
		}
		if !task.allowedBy(policy, time.Now()) {
			respondWithError(c, http.StatusConflict, "task is not active for its agent (status "+string(task.Status)+")")
			return
		}
		c.Set("authorized_agent_id", *task.AgentID)
		c.Next()
	}
}

// RequireWorkspaceAgentTask binds a workspace route (after
// PoolTokenAuthWithWorkspaceCheck) to an agent of the calling pool that is
// executing (or, within the grace, has just ended) a task on that workspace.
// With an agent token the task must be that agent's.
func RequireWorkspaceAgentTask(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		poolID := c.GetString("pool_id")
		workspaceID := c.GetString("authorized_workspace_id")
		if poolID == "" || workspaceID == "" {
			respondWithError(c, http.StatusInternalServerError, "workspace authorization context missing")
			return
		}
		now := time.Now()
		q := db.WithContext(c.Request.Context()).Table("workspace_tasks AS t").
			Select("t.id, t.agent_id, t.status, t.completed_at, t.updated_at").
			Joins("JOIN agents a ON a.agent_id = t.agent_id").
			Where("t.workspace_id = ? AND a.pool_id = ?", workspaceID, poolID).
			Where("(t.status = ? OR (t.status IN ? AND COALESCE(t.completed_at, t.updated_at) >= ?))",
				models.TaskStatusRunning, agentReportStatuses, now.Add(-AgentTaskReportGrace))
		if tokAgent := TokenAgentID(c); tokAgent != "" {
			q = q.Where("t.agent_id = ?", tokAgent)
		}
		var tasks []agentTaskRow
		if err := q.Order("t.id DESC").Limit(20).Find(&tasks).Error; err != nil {
			respondWithError(c, http.StatusInternalServerError, "Failed to query workspace tasks")
			return
		}
		for i := range tasks {
			t := &tasks[i]
			if t.AgentID == nil || !t.allowedBy(AgentTaskReporting, now) {
				continue
			}
			err := verifyTaskAgent(db, c, poolID, *t.AgentID)
			if err == nil {
				c.Set("authorized_agent_id", *t.AgentID)
				c.Set("authorized_task_id", t.ID)
				c.Next()
				return
			}
			if !errors.Is(err, errAgentNotOwner) {
				respondWithError(c, http.StatusInternalServerError, "Failed to verify task agent")
				return
			}
		}
		log.Printf("[AgentAuth] [WARN] pool %s caller %q has no active task on workspace %s",
			poolID, TokenAgentID(c), workspaceID)
		respondWithError(c, http.StatusForbidden, "no active task of the calling agent on this workspace")
	}
}
