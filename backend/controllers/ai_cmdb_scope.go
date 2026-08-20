package controllers

import (
	"fmt"
	"strconv"
	"strings"

	"iac-platform/internal/middleware"
	"iac-platform/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// aiCMDBTenantScope is the controller-side representation of one IAM-bound
// AI/CMDB request. organizationID is deliberately derived from auth_org_id,
// never from context_ids supplied by the client.
type aiCMDBTenantScope struct {
	organizationID string
	workspaceID    string
	cmdbScope      services.CMDBWorkspaceScope
}

func resolveAICMDBTenantScope(
	ctx *gin.Context,
	db *gorm.DB,
	requestedOrganizationID string,
	requestedWorkspaceID string,
) (aiCMDBTenantScope, error) {
	authOrgID, ok := middleware.AuthOrgID(ctx)
	if !ok || authOrgID == 0 {
		return aiCMDBTenantScope{}, fmt.Errorf("missing authenticated organization")
	}

	if rawOrgID := strings.TrimSpace(requestedOrganizationID); rawOrgID != "" {
		parsedOrgID, err := strconv.ParseUint(rawOrgID, 10, 64)
		if err != nil || parsedOrgID == 0 || parsedOrgID > uint64(^uint(0)) || uint(parsedOrgID) != authOrgID {
			return aiCMDBTenantScope{}, fmt.Errorf("request organization does not match authenticated organization")
		}
	}

	cmdbScope, err := services.ResolveCMDBWorkspaceScope(db, authOrgID, requestedWorkspaceID)
	if err != nil {
		// Do not reveal whether a submitted workspace exists in another org.
		return aiCMDBTenantScope{}, fmt.Errorf("invalid workspace context")
	}

	return aiCMDBTenantScope{
		organizationID: strconv.FormatUint(uint64(authOrgID), 10),
		workspaceID:    requestedWorkspaceID,
		cmdbScope:      cmdbScope,
	}, nil
}
