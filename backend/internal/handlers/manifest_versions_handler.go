package handlers

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"iac-platform/internal/models"
)

// ManifestVersionsHandler 处理 manifest 版本读 / 发布 / diff / export
//
// 路由:
//   GET  /manifests/:id/versions                                列表
//   GET  /manifests/:id/versions/:version_id                    详情(只读)
//   POST /manifests/:id/versions                                发布: 从当前用户草稿快照成 vX.Y.Z
//   GET  /manifests/:id/versions/:version_id/diff?against=:b    版本 diff
//   POST /manifests/:id/versions/:version_id/files/_export      导出版本所有文件为 zip
type ManifestVersionsHandler struct {
	db *gorm.DB
}

func NewManifestVersionsHandler(db *gorm.DB) *ManifestVersionsHandler {
	return &ManifestVersionsHandler{db: db}
}

var semverPattern = regexp.MustCompile(`^v\d+\.\d+\.\d+$`)

// ListVersions 已发布版本列表
func (h *ManifestVersionsHandler) ListVersions(c *gin.Context) {
	manifestID := c.Param("id")

	var versions []models.ManifestVersion
	if err := h.db.Where("manifest_id = ?", manifestID).
		Where("version <> ?", "draft").
		Order("created_at DESC").
		Find(&versions).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 按 SemVer 排序(数字最大者在前)
	sort.Slice(versions, func(i, j int) bool {
		return compareSemver(versions[i].Version, versions[j].Version) > 0
	})

	c.JSON(http.StatusOK, gin.H{"versions": versions})
}

// GetVersion 版本详情
func (h *ManifestVersionsHandler) GetVersion(c *gin.Context) {
	manifestID := c.Param("id")
	versionID := c.Param("version_id")

	var v models.ManifestVersion
	if err := h.db.Where("id = ? AND manifest_id = ?", versionID, manifestID).First(&v).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "version not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}
	c.JSON(http.StatusOK, v)
}

// PublishVersion 把当前用户草稿快照为新版本
func (h *ManifestVersionsHandler) PublishVersion(c *gin.Context) {
	manifestID := c.Param("id")
	userID := c.GetString("user_id")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user_id missing"})
		return
	}

	var req models.PublishVersionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !semverPattern.MatchString(req.Version) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "version must match vX.Y.Z (e.g. v1.2.0)"})
		return
	}

	// 校验 manifest 存在
	var n int64
	h.db.Model(&models.Manifest{}).Where("id = ?", manifestID).Count(&n)
	if n == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "manifest not found"})
		return
	}

	// 校验同 manifest 下 version 不重复
	var dup int64
	h.db.Model(&models.ManifestVersion{}).
		Where("manifest_id = ? AND version = ?", manifestID, req.Version).Count(&dup)
	if dup > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "version already exists"})
		return
	}

	// 校验当前用户草稿至少有一个 .tf
	var draftCount int64
	h.db.Model(&models.ManifestFile{}).
		Where("manifest_id = ? AND owner_user_id = ? AND version_id IS NULL", manifestID, userID).
		Where("path LIKE ?", "%.tf").Count(&draftCount)
	if draftCount == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "draft must contain at least one .tf file before publishing"})
		return
	}

	newVersionID := generateManifestVersionID()
	hclParseFailed := false

	err := h.db.Transaction(func(tx *gorm.DB) error {
		// 1. 写 manifest_versions
		v := models.ManifestVersion{
			ID:         newVersionID,
			ManifestID: manifestID,
			Version:    req.Version,
			Changelog:  req.Changelog,
			IsDraft:    false,
			CreatedBy:  userID,
			CreatedAt:  time.Now(),
		}
		if err := tx.Create(&v).Error; err != nil {
			return err
		}

		// 2. 全量复制草稿到该 version_id
		// PostgreSQL 默认 READ COMMITTED: 这个 SELECT 看到事务开始时刻的草稿快照,
		// 同时段独立事务的 PUT 不会污染这次快照(已记入 spec §8.2)
		if err := tx.Exec(`
			INSERT INTO manifest_files (manifest_id, version_id, owner_user_id, path, content, mime, size, is_binary, mode, created_at, updated_at)
			SELECT manifest_id, ?, NULL, path, content, mime, size, is_binary, mode, NOW(), NOW()
			FROM manifest_files
			WHERE manifest_id = ? AND version_id IS NULL AND owner_user_id = ?
		`, newVersionID, manifestID, userID).Error; err != nil {
			return err
		}

		// 3. (best-effort) HCL 静态解析提取 input variables 元信息
		// 失败不阻塞发布(spec §8.2)
		// TODO(PR3): 实现 extractVariablesMetadata,把结果写入 v.Variables
		_ = hclParseFailed

		return nil
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	resp := gin.H{
		"id":         newVersionID,
		"version":    req.Version,
		"changelog":  req.Changelog,
		"created_by": userID,
		"created_at": time.Now(),
	}
	if hclParseFailed {
		resp["warning"] = "HCL parse failed, variables metadata not extracted"
	}
	c.JSON(http.StatusCreated, resp)
}

// ExportVersion 导出某 version 全部文件为 zip
func (h *ManifestVersionsHandler) ExportVersion(c *gin.Context) {
	manifestID := c.Param("id")
	versionID := c.Param("version_id")

	var rows []models.ManifestFile
	if err := h.db.Where("manifest_id = ? AND version_id = ?", manifestID, versionID).
		Order("path ASC").Find(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)
	for _, f := range rows {
		w, err := zw.Create(f.Path)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if _, err := w.Write(f.Content); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	if err := zw.Close(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Header("Content-Type", "application/zip")
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s-%s.zip"`, manifestID, versionID))
	c.Data(http.StatusOK, "application/zip", buf.Bytes())
}

// DiffVersions 简单文件级 diff: 返回两版本的文件列表差异(不做内容 line diff,前端 Monaco 自己做)
func (h *ManifestVersionsHandler) DiffVersions(c *gin.Context) {
	manifestID := c.Param("id")
	versionID := c.Param("version_id")
	against := c.Query("against")
	if against == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "against required"})
		return
	}

	var aFiles, bFiles []models.ManifestFile
	h.db.Select("path, mime, size, is_binary").
		Where("manifest_id = ? AND version_id = ?", manifestID, versionID).
		Order("path ASC").Find(&aFiles)
	h.db.Select("path, mime, size, is_binary").
		Where("manifest_id = ? AND version_id = ?", manifestID, against).
		Order("path ASC").Find(&bFiles)

	aMap := make(map[string]models.ManifestFile, len(aFiles))
	for _, f := range aFiles {
		aMap[f.Path] = f
	}
	bMap := make(map[string]models.ManifestFile, len(bFiles))
	for _, f := range bFiles {
		bMap[f.Path] = f
	}

	type entry struct {
		Path  string `json:"path"`
		State string `json:"state"` // added | removed | maybe_changed
	}
	var added, removed, maybe []entry
	for p := range aMap {
		if _, ok := bMap[p]; !ok {
			added = append(added, entry{Path: p, State: "added"})
		} else {
			maybe = append(maybe, entry{Path: p, State: "maybe_changed"})
		}
	}
	for p := range bMap {
		if _, ok := aMap[p]; !ok {
			removed = append(removed, entry{Path: p, State: "removed"})
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"added":         added,
		"removed":       removed,
		"maybe_changed": maybe,
	})
}

// =============================================================================
// helpers
// =============================================================================

// compareSemver 比较 vX.Y.Z 字符串,返回 a-b(>0 表示 a 更大)
func compareSemver(a, b string) int {
	pa := parseSemver(a)
	pb := parseSemver(b)
	for i := 0; i < 3; i++ {
		if pa[i] != pb[i] {
			return pa[i] - pb[i]
		}
	}
	return 0
}

func parseSemver(s string) [3]int {
	var out [3]int
	if !semverPattern.MatchString(s) {
		return out
	}
	s = s[1:] // strip 'v'
	parts := splitN(s, '.', 3)
	for i := 0; i < 3 && i < len(parts); i++ {
		n := 0
		for _, c := range parts[i] {
			if c < '0' || c > '9' {
				return [3]int{}
			}
			n = n*10 + int(c-'0')
		}
		out[i] = n
	}
	return out
}

func splitN(s string, sep byte, n int) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == sep {
			out = append(out, s[start:i])
			start = i + 1
			if len(out) == n-1 {
				out = append(out, s[start:])
				return out
			}
		}
	}
	out = append(out, s[start:])
	return out
}
