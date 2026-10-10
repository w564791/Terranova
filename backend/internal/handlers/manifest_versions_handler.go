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
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"iac-platform/internal/gitsource"
	"iac-platform/internal/manifestbundle"
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
// @Summary List manifest versions
// @Description List published versions for a manifest (SemVer descending). Each version carries the stored bundle_hash (null when the version has no valid bundle) and bundle_invalid_reason (rule names and paths, or hash_mismatch; null when valid). Read-only: hashes are never recomputed here.
// @Tags Manifest Versions
// @Accept json
// @Produce json
// @Param org_id path string true "Organization ID"
// @Param id path string true "Manifest ID"
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/organizations/{org_id}/manifests/{id}/v2/versions [get]
// @Security BearerAuth
func (h *ManifestVersionsHandler) ListVersions(c *gin.Context) {
	manifestID := c.Param("id")

	var versions []models.ManifestVersion
	if err := h.db.Where("manifest_id = ?", manifestID).
		Where("version <> ?", "draft").
		Order("created_at DESC").
		Find(&versions).Error; err != nil {
		_ = c.Error(err)
		return
	}

	// 按 SemVer 排序(数字最大者在前)
	sort.Slice(versions, func(i, j int) bool {
		return compareSemver(versions[i].Version, versions[j].Version) > 0
	})

	c.JSON(http.StatusOK, gin.H{"versions": versions})
}

// GetVersion 版本详情
// @Summary Get manifest version
// @Description Get a published version detail by version ID, including bundle_hash and bundle_invalid_reason
// @Tags Manifest Versions
// @Accept json
// @Produce json
// @Param org_id path string true "Organization ID"
// @Param id path string true "Manifest ID"
// @Param version_id path string true "Version ID"
// @Success 200 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/organizations/{org_id}/manifests/{id}/v2/versions/{version_id} [get]
// @Security BearerAuth
func (h *ManifestVersionsHandler) GetVersion(c *gin.Context) {
	manifestID := c.Param("id")
	versionID := c.Param("version_id")

	var v models.ManifestVersion
	if err := h.db.Where("id = ? AND manifest_id = ?", versionID, manifestID).First(&v).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "version not found"})
		} else {
			_ = c.Error(err)
		}
		return
	}
	c.JSON(http.StatusOK, v)
}

// ListWorkdirs 列出某版本里所有"直接含 .tf 文件"的目录(去重升序,根用 "")。
// 供部署 install 时让用户选 terraform 执行子目录(workdir)。
// @Summary List version workdirs
// @Description List directories that directly contain .tf files for install workdir selection
// @Tags Manifest Versions
// @Accept json
// @Produce json
// @Param org_id path string true "Organization ID"
// @Param id path string true "Manifest ID"
// @Param version_id path string true "Version ID"
// @Success 200 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/organizations/{org_id}/manifests/{id}/v2/versions/{version_id}/workdirs [get]
// @Security BearerAuth
func (h *ManifestVersionsHandler) ListWorkdirs(c *gin.Context) {
	manifestID := c.Param("id")
	versionID := c.Param("version_id")

	bundle, ok := openVersionBundle(c, h.db, manifestID, versionID)
	if !ok {
		return
	}
	scope := make(map[string][]byte, len(bundle.Files))
	for _, f := range bundle.Files {
		if strings.HasSuffix(f.Path, ".tf") {
			scope[f.Path] = nil // 目录推断不需要内容
		}
	}
	c.JSON(http.StatusOK, gin.H{"workdirs": services.ListWorkdirs(scope)})
}

// PublishVersion 把当前用户草稿快照为新版本
// @Summary Publish manifest version
// @Description Snapshot the current user's draft (native manifests) or the commit commit_sha of the repository (git manifests: required there, refused for native; the platform fetches that commit with a per-publish GitHub App installation token, single repo contents:read, and the version records source_ref = the SHA; symlinks and submodules are rejected as git_symlink / git_submodule problems; changelog defaults to the commit subject) into a new published version (vX.Y.Z). Git module sources in the bundle must pin a full commit SHA (?ref=<40-hex>) or be vendored: hcl_module_unpinned. Git errors: 422 git_repo_not_accessible / git_commit_not_found / git_subpath_not_found, 502 git_fetch_failed, 503 git_source_disabled. The draft is packed into an immutable bundle and the response includes bundle_hash. A draft that breaks the bundle rules is rejected with 422 bundle_rules_violated; each problem is {file, line?, rule, message} and never contains file content. Besides the path / denylist / size / secret-scan rules, every Terraform configuration file (*.tf, *.tf.json, *_override.tf[.json], *.tofu[.json]) is statically checked: hcl_parse_error (unparsable file), hcl_provisioner (any provisioner block), hcl_external_data / hcl_http_data (data "external" / data "http", or required_providers mapping hashicorp/external / hashicorp/http), hcl_module_source (module source that is not a relative path inside the bundle nor an active platform module catalog source). line is set for secret-scan and HCL problems (1-based line of the hit / block / attribute).
// @Tags Manifest Versions
// @Accept json
// @Produce json
// @Param org_id path string true "Organization ID"
// @Param id path string true "Manifest ID"
// @Param request body models.PublishVersionRequest true "Publish payload"
// @Success 201 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 409 {object} map[string]interface{}
// @Failure 422 {object} handlers.BundleRulesViolatedResponse
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/organizations/{org_id}/manifests/{id}/v2/versions [post]
// @Security BearerAuth
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
	var manifest models.Manifest
	if err := h.db.Where("id = ?", manifestID).Take(&manifest).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "manifest not found"})
		} else {
			_ = c.Error(err)
		}
		return
	}
	isGit := manifest.SourceType == models.ManifestSourceGit
	switch {
	case isGit && !gitsource.IsCommitSHA(req.CommitSHA):
		c.JSON(http.StatusBadRequest, gin.H{"error": "commit_sha (full lowercase 40 or 64 hex commit id) is required to publish a git manifest"})
		return
	case !isGit && req.CommitSHA != "":
		c.JSON(http.StatusBadRequest, gin.H{"error": "commit_sha is only accepted for git manifests"})
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

	newVersionID := generateManifestVersionID()

	// git: fetch the pinned commit platform-side, outside the transaction
	// (network); runs never fetch git, they use the stored bundle.
	var gitTree *gitsource.Tree
	if isGit {
		tree, err := fetchGitCommit(c.Request.Context(), &manifest, req.CommitSHA)
		if err != nil {
			respondGitError(c, "publish "+manifestID, err)
			return
		}
		gitTree = tree
		if req.Changelog == "" {
			req.Changelog = tree.Subject
		}
	}

	// 发布 = 把当前用户草稿打包成不可变 bundle(manifestbundle.Pack:规则校验 + Hash),
	// 在同一事务里读草稿、写版本行、存 bundle(manifest_files 版本快照行)并写 bundle_hash。
	// 变量元信息也从同一份文件集提取,与 bundle 内容一致。
	var (
		problems       []manifestbundle.Problem
		noTF           bool
		bundleHash     string
		hclParseFailed bool
	)
	errRejected := errors.New("publish rejected")
	err := h.db.Transaction(func(tx *gorm.DB) error {
		ctx := c.Request.Context()
		var src manifestbundle.Source = manifestbundle.NativeDraft{DB: tx, ManifestID: manifestID, OwnerUserID: userID}
		if gitTree != nil {
			src = manifestbundle.GitCommit{SHA: gitTree.SHA, Files: gitTree.Files}
		}
		files, err := src.ReadFiles(ctx)
		if err != nil {
			return err
		}
		if !hasTFFile(files) {
			noTF = true
			return errRejected
		}
		// 发布规则 = bundle 规则 + HCL 静态检查(provisioner / data external|http /
		// module source 白名单,见 manifestbundle.CheckHCL);白名单唯一入口
		// PublishModuleSourcePolicy(本地相对路径 + 平台 module 目录里的 module_source)。
		bundle, probs, err := manifestbundle.PackFilesForPublish(files, manifestbundle.PublishModuleSourcePolicy(ctx, tx))
		if err != nil {
			return err
		}
		if gitTree != nil {
			// symlinks / submodules / oversize entries the fetcher did not read
			probs = gitCommitProblems(gitTree, probs)
		}
		if len(probs) > 0 {
			problems = probs
			return errRejected
		}
		bundleHash = bundle.Hash

		// (best-effort) HCL 静态解析提取 input variables 元信息(spec §7.6/§8.2),失败不阻塞发布
		var variablesJSON json.RawMessage
		variablesJSON, hclParseFailed = variablesJSONFromScope(bundle.Scope())

		v := models.ManifestVersion{
			ID:         newVersionID,
			ManifestID: manifestID,
			Version:    req.Version,
			Changelog:  req.Changelog,
			Variables:  variablesJSON,
			CreatedBy:  userID,
			CreatedAt:  time.Now(),
		}
		if gitTree != nil {
			sha := gitTree.SHA
			v.SourceRef = &sha // the version is pinned to this commit
		}
		if err := tx.Create(&v).Error; err != nil {
			return err
		}
		if err := manifestbundle.Store(ctx, tx, manifestID, newVersionID, bundle); err != nil {
			return err
		}

		// 首次发布后把 manifest 状态从 draft 置为 published(列表页据此显示状态)
		return tx.Model(&models.Manifest{}).
			Where("id = ? AND status = ?", manifestID, models.ManifestStatusDraft).
			Update("status", models.ManifestStatusPublished).Error
	})

	switch {
	case noTF && isGit:
		c.JSON(http.StatusBadRequest, gin.H{"error": "the commit has no .tf file under git_subpath"})
		return
	case noTF:
		c.JSON(http.StatusBadRequest, gin.H{"error": "draft must contain at least one .tf file before publishing"})
		return
	case len(problems) > 0:
		msg := "draft violates the bundle rules"
		if isGit {
			msg = "commit violates the bundle rules"
		}
		// 422 problems: 每项只有规则名与路径,不含文件内容或命中文本
		c.JSON(http.StatusUnprocessableEntity, BundleRulesViolatedResponse{
			Error:    msg,
			Code:     "bundle_rules_violated",
			Problems: problems,
		})
		return
	case err != nil:
		_ = c.Error(err)
		return
	}

	audit := map[string]interface{}{
		"manifest_id": manifestID,
		"version_id":  newVersionID,
		"version":     req.Version,
		"changelog":   req.Changelog,
		"bundle_hash": bundleHash,
	}
	if gitTree != nil {
		audit["source_ref"] = gitTree.SHA
	}
	writeManifestAudit(h.db, auditResourceManifestVersion, "version.publish", userID, audit)

	resp := gin.H{
		"id":         newVersionID,
		"version":    req.Version,
		"changelog":  req.Changelog,
		"created_by":  userID,
		"created_at":  time.Now(),
		"bundle_hash": bundleHash,
	}
	if gitTree != nil {
		resp["source_ref"] = gitTree.SHA
	}
	if hclParseFailed {
		resp["warning"] = "HCL parse failed, variables metadata not extracted"
	}
	c.JSON(http.StatusCreated, resp)
}

// hasTFFile 文件集中至少有一个 .tf
func hasTFFile(files []manifestbundle.File) bool {
	for _, f := range files {
		if strings.HasSuffix(f.Path, ".tf") {
			return true
		}
	}
	return false
}

// variablesJSONFromScope 浅 parse .tf 中的 variable block,返回 (variables 元信息 JSON, 是否解析出错)。
//
// best-effort 语义:失败返回 (nil, true),由调用方决定是否在响应里加 warning,
// 不阻塞发布。无 variable 声明时返回 ("[]", false)。只扫 .tf,二进制 / .tfvars 等不参与。
func variablesJSONFromScope(all map[string][]byte) (json.RawMessage, bool) {
	scope := make(map[string][]byte, len(all))
	for p, content := range all {
		if strings.HasSuffix(p, ".tf") {
			scope[p] = content
		}
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
// @Summary Export version ZIP
// @Description Export all files of a published version as a ZIP archive
// @Tags Manifest Versions
// @Accept json
// @Produce application/zip
// @Param org_id path string true "Organization ID"
// @Param id path string true "Manifest ID"
// @Param version_id path string true "Version ID"
// @Success 200 {file} binary "ZIP archive"
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/organizations/{org_id}/manifests/{id}/v2/versions/{version_id}/files/_export [post]
// @Security BearerAuth
func (h *ManifestVersionsHandler) ExportVersion(c *gin.Context) {
	manifestID := c.Param("id")
	versionID := c.Param("version_id")

	bundle, ok := openVersionBundle(c, h.db, manifestID, versionID)
	if !ok {
		return
	}
	buf, err := zipFiles(bundle.Files)
	if err != nil {
		_ = c.Error(err)
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
func computeFileDiff(targetFiles, baseFiles []manifestbundle.File) []diffEntry {
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
// @Summary Diff two versions
// @Description File-level content diff of target version against another version
// @Tags Manifest Versions
// @Accept json
// @Produce json
// @Param org_id path string true "Organization ID"
// @Param id path string true "Manifest ID"
// @Param version_id path string true "Target version ID"
// @Param against query string true "Base version ID to compare against"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Router /api/v1/organizations/{org_id}/manifests/{id}/v2/versions/{version_id}/diff [get]
// @Security BearerAuth
func (h *ManifestVersionsHandler) DiffVersions(c *gin.Context) {
	manifestID := c.Param("id")
	versionID := c.Param("version_id")
	against := c.Query("against")
	if against == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "against required"})
		return
	}

	a, ok := openVersionBundle(c, h.db, manifestID, versionID)
	if !ok {
		return
	}
	b, ok := openVersionBundle(c, h.db, manifestID, against)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, gin.H{"files": computeFileDiff(a.Files, b.Files)})
}

// DiffDraft 当前用户草稿 vs 某版本(默认最新已发布)的文件级真内容比对(target=草稿 相对 base=版本)
// GET /manifests/:id/v2/draft/diff?against=<version_id>  (against 省略时取最新已发布版本)
// @Summary Diff draft against version
// @Description File-level content diff of current user draft against a version (defaults to latest published)
// @Tags Manifest Versions
// @Accept json
// @Produce json
// @Param org_id path string true "Organization ID"
// @Param id path string true "Manifest ID"
// @Param against query string false "Base version ID (defaults to latest published)"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/organizations/{org_id}/manifests/{id}/v2/draft/diff [get]
// @Security BearerAuth
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

	// 草稿侧是编辑器自己的草稿(NativeDraft);基线版本侧读不可变 bundle
	draftFiles, err := manifestbundle.NativeDraft{DB: h.db, ManifestID: manifestID, OwnerUserID: userID}.ReadFiles(c.Request.Context())
	if err != nil {
		_ = c.Error(err)
		return
	}
	var baseFiles []manifestbundle.File
	if baseVersionID != "" {
		base, ok := openVersionBundle(c, h.db, manifestID, baseVersionID)
		if !ok {
			return
		}
		baseFiles = base.Files
	}

	c.JSON(http.StatusOK, gin.H{
		"base_version_id": baseVersionID,
		"files":           computeFileDiff(draftFiles, baseFiles),
	})
}

// =============================================================================
// helpers
// =============================================================================

// openVersionBundle 只读接口(导出 / diff / 目录选择)读已发布版本的 bundle:只读存储的
// 文件与 bundle_hash,不重算哈希、不写库(完整性只在真正使用版本处校验,见
// openDeployableBundle)。无合法 bundle 的版本仍可读。版本不存在 => 404;数据库错误 => 500。
func openVersionBundle(c *gin.Context, db *gorm.DB, manifestID, versionID string) (*manifestbundle.Bundle, bool) {
	bundle, err := manifestbundle.OpenVersion(c.Request.Context(), db, manifestID, versionID)
	switch {
	case err == nil:
		return bundle, true
	case errors.Is(err, manifestbundle.ErrVersionNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "version not found"})
	default:
		_ = c.Error(err)
	}
	return nil, false
}

// zipFiles 按 path 顺序把文件写成 zip
func zipFiles(files []manifestbundle.File) (*bytes.Buffer, error) {
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)
	for _, f := range files {
		w, err := zw.Create(f.Path)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(f.Content); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf, nil
}

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
