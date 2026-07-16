package controllers

import (
	"log"
	"net/http"
	"strings"
	"time"

	"iac-platform/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type VariableSnapshotController struct {
	service *services.VariableSnapshotService
}

func NewVariableSnapshotController(db *gorm.DB) *VariableSnapshotController {
	return &VariableSnapshotController{
		service: services.NewVariableSnapshotService(db),
	}
}

// CreateSnapshot POST /workspaces/:id/variable-snapshots
// @Summary Create variable snapshot
// @Description Create a variable snapshot for a workspace
// @Tags Workspace Variable
// @Accept json
// @Produce json
// @Param id path string true "Workspace ID"
// @Success 201 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Security BearerAuth
// @Router /api/v1/workspaces/{id}/variable-snapshots [post]
func (c *VariableSnapshotController) CreateSnapshot(ctx *gin.Context) {
	workspaceID := ctx.Param("id")
	var userID *string
	if uid, exists := ctx.Get("user_id"); exists {
		s := uid.(string)
		userID = &s
	}

	vsnapID, count, err := c.service.CreateSnapshot(workspaceID, userID)
	if err != nil {
		log.Printf("Failed to create variable snapshot: %v", err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create snapshot"})
		return
	}

	ctx.JSON(http.StatusCreated, gin.H{
		"vsnap_id":     vsnapID,
		"workspace_id": workspaceID,
		"item_count":   count,
		"created_at":   time.Now(),
	})
}

// DeleteSnapshot DELETE /workspaces/:id/variable-snapshots/:vsnap_id
// @Summary Delete variable snapshot
// @Description Delete a variable snapshot from a workspace
// @Tags Workspace Variable
// @Accept json
// @Produce json
// @Param id path string true "Workspace ID"
// @Param vsnap_id path string true "Variable snapshot ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Security BearerAuth
// @Router /api/v1/workspaces/{id}/variable-snapshots/{vsnap_id} [delete]
func (c *VariableSnapshotController) DeleteSnapshot(ctx *gin.Context) {
	vsnapID := ctx.Param("vsnap_id")
	if vsnapID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "vsnap_id is required"})
		return
	}

	if err := c.service.DeleteSnapshot(vsnapID); err != nil {
		if strings.Contains(err.Error(), "not found") {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		} else {
			log.Printf("Failed to delete snapshot: %v", err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete snapshot"})
		}
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "deleted"})
}
