package handlers

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"iac-platform/internal/models"
)

// ManifestFilesHandler 处理 manifest 文件 CRUD(草稿区与版本快照只读)
//
// 路由(挂在 /api/v1/organizations/:org_id):
//   GET    /manifests/:id/files                            列出当前用户草稿文件树
//   GET    /manifests/:id/files?version=<id>               列出某 published 版本的文件树(只读)
//   GET    /manifests/:id/files/*path                      读单文件
//   GET    /manifests/:id/files/*path?version=<id>         读 published 版本的单文件(只读)
//   PUT    /manifests/:id/files/*path                      写当前用户草稿
//   DELETE /manifests/:id/files/*path                      删当前用户草稿
//   POST   /manifests/:id/files/_move                      重命名/移动
//   POST   /manifests/:id/draft/_reset_from?version_id=    用 published 版本覆盖草稿
//   POST   /manifests/:id/draft/_export                    导出当前用户草稿为 zip
type ManifestFilesHandler struct {
	db *gorm.DB
}

func NewManifestFilesHandler(db *gorm.DB) *ManifestFilesHandler {
	return &ManifestFilesHandler{db: db}
}

// 文件大小硬上限(可被中间件覆盖,这里再做一次防御性校验)
const ManifestMaxFileSize = 1 * 1024 * 1024 // 1 MB

// ListFiles 返回文件树。version 查询参数: 空 = 草稿; <version_id> = 已发布版本只读
func (h *ManifestFilesHandler) ListFiles(c *gin.Context) {
	manifestID := c.Param("id")
	versionID := c.Query("version")
	userID := c.GetString("user_id")

	if !h.manifestExists(manifestID) {
		c.JSON(http.StatusNotFound, gin.H{"error": "manifest not found"})
		return
	}

	q := h.db.Model(&models.ManifestFile{}).
		Select("path, mime, size, is_binary").
		Where("manifest_id = ?", manifestID)

	if versionID == "" || versionID == "draft" {
		// 当前用户私有草稿;若不存在,首次按 latest published 初始化(ON CONFLICT DO NOTHING)
		if err := h.ensureDraftInitialized(manifestID, userID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		q = q.Where("version_id IS NULL").Where("owner_user_id = ?", userID)
	} else {
		q = q.Where("version_id = ?", versionID)
	}

	var rows []models.ManifestFile
	if err := q.Order("path ASC").Find(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	tree := make([]models.ManifestFileTreeEntry, 0, len(rows))
	for _, r := range rows {
		tree = append(tree, models.ManifestFileTreeEntry{
			Path:     r.Path,
			Name:     filepath.Base(r.Path),
			Type:     "file",
			Size:     r.Size,
			Mime:     r.Mime,
			IsBinary: r.IsBinary,
		})
	}
	c.JSON(http.StatusOK, gin.H{"files": tree})
}

// ReadFile 读单文件内容。binary 文件以 base64 形式返回
func (h *ManifestFilesHandler) ReadFile(c *gin.Context) {
	manifestID := c.Param("id")
	rawPath := c.Param("path")
	versionID := c.Query("version")
	userID := c.GetString("user_id")

	path, err := normalizeAndValidatePath(rawPath)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	q := h.db.Where("manifest_id = ? AND path = ?", manifestID, path)
	if versionID == "" || versionID == "draft" {
		q = q.Where("version_id IS NULL").Where("owner_user_id = ?", userID)
	} else {
		q = q.Where("version_id = ?", versionID)
	}

	var row models.ManifestFile
	if err := q.First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	resp := gin.H{
		"path":       row.Path,
		"size":       row.Size,
		"mime":       row.Mime,
		"is_binary":  row.IsBinary,
		"updated_at": row.UpdatedAt,
	}
	if row.IsBinary {
		resp["content_b64"] = base64.StdEncoding.EncodeToString(row.Content)
	} else {
		resp["content"] = string(row.Content)
	}
	c.JSON(http.StatusOK, resp)
}

// PutFile 写当前用户草稿
func (h *ManifestFilesHandler) PutFile(c *gin.Context) {
	manifestID := c.Param("id")
	rawPath := c.Param("path")
	userID := c.GetString("user_id")

	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user_id missing"})
		return
	}

	path, err := normalizeAndValidatePath(rawPath)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var req struct {
		Content    string `json:"content"`     // 文本路径
		ContentB64 string `json:"content_b64"` // 二进制路径
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var raw []byte
	if req.ContentB64 != "" {
		decoded, err := base64.StdEncoding.DecodeString(req.ContentB64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid base64: " + err.Error()})
			return
		}
		raw = decoded
	} else {
		raw = []byte(req.Content)
	}

	if len(raw) > ManifestMaxFileSize {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": fmt.Sprintf("file exceeds %d bytes", ManifestMaxFileSize)})
		return
	}

	mime, isBinary := sniffContent(path, raw)

	if !h.manifestExists(manifestID) {
		c.JSON(http.StatusNotFound, gin.H{"error": "manifest not found"})
		return
	}

	row := models.ManifestFile{
		ManifestID:  manifestID,
		VersionID:   nil,
		OwnerUserID: &userID,
		Path:        path,
		Content:     raw,
		Mime:        mime,
		Size:        len(raw),
		IsBinary:    isBinary,
		Mode:        420,
		UpdatedAt:   time.Now(),
	}

	// UPSERT 走部分唯一索引 uq_mf_draft (manifest_id, owner_user_id, path) WHERE version_id IS NULL
	// 必须用 TargetWhere(= ON CONFLICT (...) WHERE <谓词>,匹配部分索引),不能用 Where。
	// Where 会落到 DO UPDATE ... WHERE,那里 version_id 同时存在于目标表与 excluded 伪表,
	// 触发 "column reference version_id is ambiguous"。
	if err := h.db.Clauses(clause.OnConflict{
		Columns:     []clause.Column{{Name: "manifest_id"}, {Name: "owner_user_id"}, {Name: "path"}},
		TargetWhere: clause.Where{Exprs: []clause.Expression{clause.Eq{Column: "version_id", Value: nil}}},
		DoUpdates: clause.AssignmentColumns([]string{
			"content", "mime", "size", "is_binary", "updated_at",
		}),
	}).Create(&row).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"path":       row.Path,
		"size":       row.Size,
		"mime":       row.Mime,
		"is_binary":  row.IsBinary,
		"updated_at": row.UpdatedAt,
	})
}

// DeleteFile 删当前用户草稿单文件
func (h *ManifestFilesHandler) DeleteFile(c *gin.Context) {
	manifestID := c.Param("id")
	rawPath := c.Param("path")
	userID := c.GetString("user_id")

	path, err := normalizeAndValidatePath(rawPath)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	res := h.db.Where("manifest_id = ? AND owner_user_id = ? AND path = ? AND version_id IS NULL",
		manifestID, userID, path).Delete(&models.ManifestFile{})
	if res.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": res.Error.Error()})
		return
	}
	if res.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": path})
}

// MoveFile 重命名 / 移动
func (h *ManifestFilesHandler) MoveFile(c *gin.Context) {
	manifestID := c.Param("id")
	userID := c.GetString("user_id")

	var req struct {
		From string `json:"from" binding:"required"`
		To   string `json:"to" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	from, err := normalizeAndValidatePath(req.From)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "from: " + err.Error()})
		return
	}
	to, err := normalizeAndValidatePath(req.To)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "to: " + err.Error()})
		return
	}
	if from == to {
		c.JSON(http.StatusBadRequest, gin.H{"error": "from == to"})
		return
	}

	// 事务: 检查 to 不存在 + 改 path
	err = h.db.Transaction(func(tx *gorm.DB) error {
		var existing int64
		tx.Model(&models.ManifestFile{}).
			Where("manifest_id = ? AND owner_user_id = ? AND path = ? AND version_id IS NULL",
				manifestID, userID, to).
			Count(&existing)
		if existing > 0 {
			return errors.New("target path already exists")
		}
		res := tx.Model(&models.ManifestFile{}).
			Where("manifest_id = ? AND owner_user_id = ? AND path = ? AND version_id IS NULL",
				manifestID, userID, from).
			Update("path", to)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "source not found"})
		} else {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"from": from, "to": to})
}

// DeleteDir 删除目录:删当前用户草稿里 path 以 "<dir>/" 开头的所有文件。
// 目录在本模型是虚拟的(无目录实体),所以删目录 = 批量删前缀下文件,一次事务。
func (h *ManifestFilesHandler) DeleteDir(c *gin.Context) {
	manifestID := c.Param("id")
	userID := c.GetString("user_id")

	var req struct {
		Dir string `json:"dir" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	dir, err := normalizeAndValidatePath(req.Dir)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// LIKE 前缀,转义 _ % \ 通配符,避免 "my_app" 误匹配 "myXapp"
	escaped := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(dir)
	pattern := escaped + "/%"

	res := h.db.Where(
		`manifest_id = ? AND owner_user_id = ? AND version_id IS NULL AND path LIKE ? ESCAPE '\'`,
		manifestID, userID, pattern,
	).Delete(&models.ManifestFile{})
	if res.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": res.Error.Error()})
		return
	}
	if res.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "directory not found or empty"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted_dir": dir, "files_removed": res.RowsAffected})
}

// MoveDir 按前缀批量移动目录:把 from/ 前缀下所有草稿文件 path 改为 to/ 前缀(事务原子)。
// 用于文件树拖拽移动文件夹。目标前缀下若已存在同名文件则整体失败(报错不覆盖)。
func (h *ManifestFilesHandler) MoveDir(c *gin.Context) {
	manifestID := c.Param("id")
	userID := c.GetString("user_id")

	var req struct {
		From string `json:"from" binding:"required"`
		To   string `json:"to" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	from, err := normalizeAndValidatePath(req.From)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "from: " + err.Error()})
		return
	}
	to, err := normalizeAndValidatePath(req.To)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "to: " + err.Error()})
		return
	}
	if from == to {
		c.JSON(http.StatusBadRequest, gin.H{"error": "from == to"})
		return
	}
	// 禁止移进自身子目录(to 以 from/ 开头会无限嵌套)
	if strings.HasPrefix(to+"/", from+"/") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot move a directory into itself"})
		return
	}

	var moved int64
	err = h.db.Transaction(func(tx *gorm.DB) error {
		// 拉 from/ 前缀下所有草稿文件
		escFrom := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(from)
		var rows []models.ManifestFile
		if err := tx.Where(
			`manifest_id = ? AND owner_user_id = ? AND version_id IS NULL AND path LIKE ? ESCAPE '\'`,
			manifestID, userID, escFrom+"/%",
		).Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return gorm.ErrRecordNotFound
		}
		// 逐个改前缀,先检查目标不存在(冲突即整体失败)
		for _, r := range rows {
			newPath := to + strings.TrimPrefix(r.Path, from)
			var existing int64
			tx.Model(&models.ManifestFile{}).
				Where("manifest_id = ? AND owner_user_id = ? AND version_id IS NULL AND path = ?",
					manifestID, userID, newPath).
				Count(&existing)
			if existing > 0 {
				return fmt.Errorf("target path already exists: %s", newPath)
			}
			if err := tx.Model(&models.ManifestFile{}).
				Where("manifest_id = ? AND owner_user_id = ? AND version_id IS NULL AND path = ?",
					manifestID, userID, r.Path).
				Update("path", newPath).Error; err != nil {
				return err
			}
			moved++
		}
		return nil
	})

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "source directory not found or empty"})
		} else {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"from": from, "to": to, "files_moved": moved})
}

// ResetDraftFromVersion 用某 published 版本覆盖当前用户草稿
func (h *ManifestFilesHandler) ResetDraftFromVersion(c *gin.Context) {
	manifestID := c.Param("id")
	versionID := c.Query("version_id")
	userID := c.GetString("user_id")

	if versionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "version_id required"})
		return
	}

	// 校验 version 存在且属于该 manifest
	var v models.ManifestVersion
	if err := h.db.Where("id = ? AND manifest_id = ?", versionID, manifestID).First(&v).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "version not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	err := h.db.Transaction(func(tx *gorm.DB) error {
		// 删旧草稿
		if err := tx.Where("manifest_id = ? AND owner_user_id = ? AND version_id IS NULL",
			manifestID, userID).Delete(&models.ManifestFile{}).Error; err != nil {
			return err
		}
		// 复制 version 的文件到草稿
		return tx.Exec(`
			INSERT INTO manifest_files (manifest_id, version_id, owner_user_id, path, content, mime, size, is_binary, mode, created_at, updated_at)
			SELECT manifest_id, NULL, ?, path, content, mime, size, is_binary, mode, NOW(), NOW()
			FROM manifest_files
			WHERE manifest_id = ? AND version_id = ?
		`, userID, manifestID, versionID).Error
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// draft.reset 是破坏性操作(用某版本整体覆盖当前用户草稿),审计。
	// 注: draft.save(PutFile)是 1s 自动保存高频私有写,无审计价值,故不逐次记录。
	writeManifestAudit(h.db, auditResourceManifestDraft, "draft.reset", userID, map[string]interface{}{
		"manifest_id": manifestID,
		"reset_from":  versionID,
		"version":     v.Version,
	})

	c.JSON(http.StatusOK, gin.H{"reset_from": versionID})
}

// ExportDraft 导出当前用户草稿为 zip
func (h *ManifestFilesHandler) ExportDraft(c *gin.Context) {
	manifestID := c.Param("id")
	userID := c.GetString("user_id")

	var rows []models.ManifestFile
	if err := h.db.Where("manifest_id = ? AND owner_user_id = ? AND version_id IS NULL",
		manifestID, userID).Order("path ASC").Find(&rows).Error; err != nil {
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
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s-draft.zip"`, manifestID))
	c.Data(http.StatusOK, "application/zip", buf.Bytes())
}

// =============================================================================
// 内部工具
// =============================================================================

func (h *ManifestFilesHandler) manifestExists(manifestID string) bool {
	var n int64
	h.db.Model(&models.Manifest{}).Where("id = ?", manifestID).Count(&n)
	return n > 0
}

// ensureDraftInitialized 首次进入时若用户没有草稿,从 latest published version
// 复制一份;并发安全(走 ON CONFLICT DO NOTHING,部分唯一索引 uq_mf_draft)。
// 若 manifest 没有任何 published version,创建空草稿(无文件,凭后续 PUT 写入)。
func (h *ManifestFilesHandler) ensureDraftInitialized(manifestID, userID string) error {
	if userID == "" {
		return errors.New("user_id missing")
	}

	// 查当前用户是否已有草稿任意一行
	var n int64
	h.db.Model(&models.ManifestFile{}).
		Where("manifest_id = ? AND owner_user_id = ? AND version_id IS NULL", manifestID, userID).
		Count(&n)
	if n > 0 {
		return nil
	}

	// latest published version
	var latest models.ManifestVersion
	res := h.db.Where("manifest_id = ?", manifestID).
		Where("version <> ?", "draft"). // 兼容旧 draft row
		Order("created_at DESC").Limit(1).Find(&latest)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		// 无 published,空草稿。下一次 PUT 写入时再插入。
		return nil
	}

	// 复制 latest 到草稿; ON CONFLICT DO NOTHING 防并发
	return h.db.Exec(`
		INSERT INTO manifest_files (manifest_id, version_id, owner_user_id, path, content, mime, size, is_binary, mode, created_at, updated_at)
		SELECT manifest_id, NULL, ?, path, content, mime, size, is_binary, mode, NOW(), NOW()
		FROM manifest_files
		WHERE manifest_id = ? AND version_id = ?
		ON CONFLICT DO NOTHING
	`, userID, manifestID, latest.ID).Error
}

// normalizeAndValidatePath 路径合法性校验
//
//   - 强制 / 作为分隔符,把 \ 转 /(兼容 Windows 用户)
//   - 禁绝对路径(以 / 开头)
//   - 禁 .. / .
//   - 字符限制 ASCII 字母数字 + - _ .
//   - 总长度 ≤ 256
//   - 末尾不带 /
func normalizeAndValidatePath(raw string) (string, error) {
	if raw == "" {
		return "", errors.New("path empty")
	}
	// 路径参数可能带前导 /
	p := strings.TrimPrefix(raw, "/")
	p = strings.ReplaceAll(p, "\\", "/")
	if p == "" {
		return "", errors.New("path empty")
	}
	if strings.HasSuffix(p, "/") {
		return "", errors.New("path must not end with /")
	}
	if len(p) > 256 {
		return "", errors.New("path too long")
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return "", errors.New("invalid path segment")
		}
		for _, r := range seg {
			if !(r == '-' || r == '_' || r == '.' ||
				(r >= '0' && r <= '9') ||
				(r >= 'a' && r <= 'z') ||
				(r >= 'A' && r <= 'Z')) {
				return "", fmt.Errorf("invalid char in path: %q", r)
			}
		}
	}
	return p, nil
}

// sniffContent 嗅探 mime 与 binary 标志
func sniffContent(path string, raw []byte) (mime string, isBinary bool) {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".tf", ".tfvars", ".hcl":
		return "text/x-terraform", false
	case ".json":
		return "application/json", false
	case ".md", ".markdown":
		return "text/markdown", false
	case ".txt":
		return "text/plain", false
	case ".tpl":
		return "text/plain", false
	case ".yaml", ".yml":
		return "application/yaml", false
	case ".sh":
		return "text/x-shellscript", false
	}
	// 未知扩展: utf-8 校验
	if utf8.Valid(raw) {
		return "text/plain", false
	}
	return "application/octet-stream", true
}
