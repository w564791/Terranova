package handlers

import (
	"net/http"

	"iac-platform/internal/reservedenv"

	"github.com/gin-gonic/gin"
)

// ReservedEnvHandler serves the read-only reserved environment-variable list.
type ReservedEnvHandler struct{}

func NewReservedEnvHandler() *ReservedEnvHandler { return &ReservedEnvHandler{} }

// ListReservedEnvPrefixes godoc
// @Summary List reserved environment variable prefixes
// @Description Returns platform-controlled env var names/prefixes that users cannot set. Any authenticated user.
// @Tags System
// @Produce json
// @Success 200 {object} map[string]interface{} "prefixes: string[]"
// @Failure 401 {object} map[string]interface{}
// @Router /api/v1/system/reserved-env-prefixes [get]
// @Security BearerAuth
func (h *ReservedEnvHandler) ListReservedEnvPrefixes(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"prefixes": reservedenv.Prefixes()})
}
