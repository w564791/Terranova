package handlers

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"iac-platform/internal/models"
	"iac-platform/services"
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

	// (best-effort) HCL 静态解析提取 input variables 元信息(spec §7.6/§8.2)
	// 失败不阻塞发布:variablesJSON 留空,前端拿不到提示但发布照常成功。
	// 只扫 .tf 文件,二进制 / .tfvars 等不参与。
	variablesJSON, hclParseFailed := extractDraftVariablesJSON(h.db, manifestID, userID)

	err := h.db.Transaction(func(tx *gorm.DB) error {
		// 1. 写 manifest_versions
		v := models.ManifestVersion{
			ID:         newVersionID,
			ManifestID: manifestID,
			Version:    req.Version,
			Changelog:  req.Changelog,
			Variables:  variablesJSON,
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

		return nil
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	writeManifestAudit(h.db, auditResourceManifestVersion, "version.publish", userID, map[string]interface{}{
		"manifest_id": manifestID,
		"version_id":  newVersionID,
		"version":     req.Version,
		"changelog":   req.Changelog,
	})

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

// extractDraftVariablesJSON 读当前用户草稿的 .tf 文件,浅 parse variable block,
// 返回 (variables 元信息 JSON, 是否解析出错)。
//
// best-effort 语义:任何一步失败都返回 (nil, true),由调用方决定是否在响应里加 warning,
// 不阻塞发布。无 variable 声明时返回 ("[]", false)。
func extractDraftVariablesJSON(db *gorm.DB, manifestID, userID string) (json.RawMessage, bool) {
	var files []models.ManifestFile
	if err := db.Select("path, content").
		Where("manifest_id = ? AND owner_user_id = ? AND version_id IS NULL", manifestID, userID).
		Where("path LIKE ?", "%.tf").
		Find(&files).Error; err != nil {
		return nil, true
	}

	scope := make(map[string][]byte, len(files))
	for _, f := range files {
		scope[f.Path] = f.Content
	}

	metas := services.ParseManifestVariables(scope)
	if metas == nil {
		metas = []services.ManifestVariableMeta{}
	}

	raw, err := json.Marshal(metas)
	if err != nil {
		return nil, true
	}
	return json.RawMessage(raw), false
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

// diffEntry 文件级变更条目(target 相对 base)
type diffEntry struct {
	Path  string `json:"path"`
	State string `json:"state"` // added | removed | changed | unchanged
}

// computeFileDiff 真内容比对两组文件(target=A 相对 base=B):
//   - added:    A 有 B 无
//   - removed:  B 有 A 无
//   - changed:  两边都有但内容不同(SHA-256 比对)
//   - unchanged:两边都有且内容相同
//
// 返回按 path 升序的扁平列表(含 unchanged,前端可自行过滤);changed 比对用 hash 避免传全量内容。
func computeFileDiff(targetFiles, baseFiles []models.ManifestFile) []diffEntry {
	hashOf := func(b []byte) string {
		s := sha256.Sum256(b)
		return hex.EncodeToString(s[:])
	}
	type meta struct{ hash string }
	aMap := make(map[string]meta, len(targetFiles))
	for _, f := range targetFiles {
		aMap[f.Path] = meta{hash: hashOf(f.Content)}
	}
	bMap := make(map[string]meta, len(baseFiles))
	for _, f := range baseFiles {
		bMap[f.Path] = meta{hash: hashOf(f.Content)}
	}

	pathSet := make(map[string]struct{}, len(aMap)+len(bMap))
	for p := range aMap {
		pathSet[p] = struct{}{}
	}
	for p := range bMap {
		pathSet[p] = struct{}{}
	}
	paths := make([]string, 0, len(pathSet))
	for p := range pathSet {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	out := make([]diffEntry, 0, len(paths))
	for _, p := range paths {
		a, inA := aMap[p]
		b, inB := bMap[p]
		switch {
		case inA && !inB:
			out = append(out, diffEntry{Path: p, State: "added"})
		case !inA && inB:
			out = append(out, diffEntry{Path: p, State: "removed"})
		case a.hash != b.hash:
			out = append(out, diffEntry{Path: p, State: "changed"})
		default:
			out = append(out, diffEntry{Path: p, State: "unchanged"})
		}
	}
	return out
}

// DiffVersions 两个已发布版本的文件级真内容比对(target=:version_id 相对 base=?against)
// GET /manifests/:id/v2/versions/:version_id/diff?against=<version_id>
func (h *ManifestVersionsHandler) DiffVersions(c *gin.Context) {
	manifestID := c.Param("id")
	versionID := c.Param("version_id")
	against := c.Query("against")
	if against == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "against required"})
		return
	}

	var aFiles, bFiles []models.ManifestFile
	h.db.Select("path, content").
		Where("manifest_id = ? AND version_id = ?", manifestID, versionID).Find(&aFiles)
	h.db.Select("path, content").
		Where("manifest_id = ? AND version_id = ?", manifestID, against).Find(&bFiles)

	c.JSON(http.StatusOK, gin.H{"files": computeFileDiff(aFiles, bFiles)})
}

// DiffDraft 当前用户草稿 vs 某版本(默认最新已发布)的文件级真内容比对(target=草稿 相对 base=版本)
// GET /manifests/:id/v2/draft/diff?against=<version_id>  (against 省略时取最新已发布版本)
func (h *ManifestVersionsHandler) DiffDraft(c *gin.Context) {
	manifestID := c.Param("id")
	userID := c.GetString("user_id")
	against := c.Query("against")

	// against 省略 → 最新已发布版本(无已发布版本则 base 为空,草稿全部算 added)
	baseVersionID := against
	if baseVersionID == "" {
		var latest models.ManifestVersion
		if err := h.db.Where("manifest_id = ? AND version <> ?", manifestID, "draft").
			Order("created_at DESC").First(&latest).Error; err == nil {
			baseVersionID = latest.ID
		}
	}

	var draftFiles, baseFiles []models.ManifestFile
	h.db.Select("path, content").
		Where("manifest_id = ? AND owner_user_id = ? AND version_id IS NULL", manifestID, userID).
		Find(&draftFiles)
	if baseVersionID != "" {
		h.db.Select("path, content").
			Where("manifest_id = ? AND version_id = ?", manifestID, baseVersionID).Find(&baseFiles)
	}

	c.JSON(http.StatusOK, gin.H{
		"base_version_id": baseVersionID,
		"files":           computeFileDiff(draftFiles, baseFiles),
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
