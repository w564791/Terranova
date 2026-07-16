package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"iac-platform/internal/models"
	"iac-platform/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ManifestProviderSchemaHandler 编辑器读取 post_init 落库的 provider 类型目录
type ManifestProviderSchemaHandler struct {
	db *gorm.DB
}

func NewManifestProviderSchemaHandler(db *gorm.DB) *ManifestProviderSchemaHandler {
	return &ManifestProviderSchemaHandler{db: db}
}

// GetProviderSchemas GET .../manifests/:id/provider-schemas?subpath=
// 返回 types 目录；无缓存时 200 + empty。
// @Summary Get provider type schemas
// @Description Return cached provider resource/data type catalog for the editor (empty when uncached)
// @Tags Manifest
// @Accept json
// @Produce json
// @Param org_id path string true "Organization ID"
// @Param id path string true "Manifest ID"
// @Param subpath query string false "Terraform workdir subpath"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/organizations/{org_id}/manifests/{id}/provider-schemas [get]
// @Security BearerAuth
func (h *ManifestProviderSchemaHandler) GetProviderSchemas(c *gin.Context) {
	manifestID := c.Param("id")
	if manifestID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "manifest id required"})
		return
	}
	subpath := c.Query("subpath")
	// 归一化与执行侧一致
	normalized, err := services.NormalizeManifestSubpath(subpath)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid subpath: " + err.Error()})
		return
	}

	var row models.ManifestProviderSchema
	err = h.db.Where("manifest_id = ? AND subpath = ? AND schema_kind = ?",
		manifestID, normalized, models.ManifestProviderSchemaKindTypes).
		First(&row).Error
	if err == gorm.ErrRecordNotFound {
		c.JSON(http.StatusOK, gin.H{
			"exists":   false,
			"manifest_id": manifestID,
			"subpath":  normalized,
			"resources": []string{},
			"data":     []string{},
			"providers": []interface{}{},
		})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var resources []string
	var data []string
	var providers []models.ProviderVersionRef
	_ = json.Unmarshal(row.Resources, &resources)
	_ = json.Unmarshal(row.DataSources, &data)
	_ = json.Unmarshal(row.Providers, &providers)
	if resources == nil {
		resources = []string{}
	}
	if data == nil {
		data = []string{}
	}
	if providers == nil {
		providers = []models.ProviderVersionRef{}
	}

	c.JSON(http.StatusOK, gin.H{
		"exists":                true,
		"manifest_id":           row.ManifestID,
		"subpath":               row.Subpath,
		"schema_kind":           row.SchemaKind,
		"providers":             providers,
		"provider_versions_key": row.ProviderVersionsKey,
		"resources":             resources,
		"data":                  data,
		"content_hash":          row.ContentHash,
		"terraform_version":     row.TerraformVersion,
		"captured_at":           row.CapturedAt.Format(time.RFC3339),
		"version":               row.ContentHash, // 状态栏短展示
	})
}
