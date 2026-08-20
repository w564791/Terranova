package handlers

import (
	"context"
	"fmt"
	"net/http"

	"iac-platform/internal/domain/valueobject"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// requireAuthOrg 读取中间件写入的 auth_org_id；缺失 → 400
// 与 application_handler.authOrgID 相同语义，供其它 handler 复用。
func requireAuthOrg(c *gin.Context) (uint, bool) {
	return authOrgID(c)
}

// respondScopeOutsideAuthOrg 统一跨 org 探测响应（403）
func respondScopeOutsideAuthOrg(c *gin.Context, err error) {
	c.JSON(http.StatusForbidden, gin.H{
		"error":   "scope outside authenticated organization",
		"details": err.Error(),
	})
}

// ensureScopeInAuthOrg 校验 ORGANIZATION/PROJECT/WORKSPACE scope 落在 authOrg 子树内。
// workspace 的 scopeID 为数字主键；库内 workspace_permissions 存语义化 workspace_id。
func ensureScopeInAuthOrg(ctx context.Context, db *gorm.DB, scopeType valueobject.ScopeType, scopeID uint, authOrg uint) error {
	if authOrg == 0 {
		return fmt.Errorf("auth org missing")
	}
	switch scopeType {
	case valueobject.ScopeTypeOrganization:
		// org 比较无需 db
		if scopeID != authOrg {
			return fmt.Errorf("org scope %d != auth %d", scopeID, authOrg)
		}
		return nil
	case valueobject.ScopeTypeProject:
		if db == nil {
			return fmt.Errorf("db not configured")
		}
		var orgID uint
		if err := db.WithContext(ctx).Table("projects").Select("org_id").Where("id = ?", scopeID).Scan(&orgID).Error; err != nil || orgID == 0 {
			return fmt.Errorf("project %d not found", scopeID)
		}
		if orgID != authOrg {
			return fmt.Errorf("project %d belongs to org %d", scopeID, orgID)
		}
		return nil
	case valueobject.ScopeTypeWorkspace:
		if db == nil {
			return fmt.Errorf("db not configured")
		}
		var wsSem string
		_ = db.WithContext(ctx).Table("workspaces").Select("workspace_id").Where("id = ?", scopeID).Scan(&wsSem)
		if wsSem == "" {
			return fmt.Errorf("workspace id %d not found", scopeID)
		}
		return ensureWorkspaceSemanticInAuthOrg(ctx, db, wsSem, authOrg)
	default:
		return fmt.Errorf("unsupported scope type %s", scopeType)
	}
}

// ensureWorkspaceSemanticInAuthOrg 语义化 workspace_id 是否属于 authOrg
func ensureWorkspaceSemanticInAuthOrg(ctx context.Context, db *gorm.DB, workspaceSemanticID string, authOrg uint) error {
	var orgID uint
	err := db.WithContext(ctx).Raw(`
SELECT p.org_id FROM workspace_project_relations wpr
JOIN projects p ON p.id = wpr.project_id
WHERE wpr.workspace_id = ? LIMIT 1`, workspaceSemanticID).Scan(&orgID).Error
	if err != nil || orgID == 0 {
		return fmt.Errorf("workspace %s has no project/org binding", workspaceSemanticID)
	}
	if orgID != authOrg {
		return fmt.Errorf("workspace belongs to org %d", orgID)
	}
	return nil
}

// resolveWorkspaceNumericToSemantic 数字主键 → 语义化 workspace_id
func resolveWorkspaceNumericToSemantic(ctx context.Context, db *gorm.DB, numericID uint) (string, error) {
	var wsSem string
	if err := db.WithContext(ctx).Table("workspaces").Select("workspace_id").Where("id = ?", numericID).Scan(&wsSem).Error; err != nil || wsSem == "" {
		return "", fmt.Errorf("workspace id %d not found", numericID)
	}
	return wsSem, nil
}

// ensureProjectBelongsToAuthOrg 项目必须属于鉴权 org
func ensureProjectBelongsToAuthOrg(projectOrgID, authOrg uint) error {
	if projectOrgID == 0 || authOrg == 0 {
		return fmt.Errorf("missing org")
	}
	if projectOrgID != authOrg {
		return fmt.Errorf("project org %d != auth %d", projectOrgID, authOrg)
	}
	return nil
}

// ensureTeamBelongsToAuthOrg 团队必须属于鉴权 org
func ensureTeamBelongsToAuthOrg(teamOrgID, authOrg uint) error {
	if teamOrgID == 0 || authOrg == 0 {
		return fmt.Errorf("missing org")
	}
	if teamOrgID != authOrg {
		return fmt.Errorf("team org %d != auth %d", teamOrgID, authOrg)
	}
	return nil
}

// requireSystemAdmin 平台超管（审计全局开关等）
func requireSystemAdmin(c *gin.Context) bool {
	if v, ok := c.Get("is_system_admin"); ok {
		if b, ok := v.(bool); ok && b {
			return true
		}
	}
	c.JSON(http.StatusForbidden, gin.H{"error": "system admin required"})
	return false
}

// loadTeamOrgID 查 teams.org_id
func loadTeamOrgID(ctx context.Context, db *gorm.DB, teamID string) (uint, error) {
	var orgID uint
	if err := db.WithContext(ctx).Table("teams").Select("org_id").Where("team_id = ?", teamID).Scan(&orgID).Error; err != nil || orgID == 0 {
		// 兼容主键 id 列
		if err2 := db.WithContext(ctx).Table("teams").Select("org_id").Where("id = ?", teamID).Scan(&orgID).Error; err2 != nil || orgID == 0 {
			return 0, fmt.Errorf("team not found")
		}
	}
	return orgID, nil
}
