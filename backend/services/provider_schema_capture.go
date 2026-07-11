package services

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"iac-platform/internal/models"
)

// postInitSchemaTimeout 限制 providers schema 提取时间，超时则跳过（不 fail task）
const postInitSchemaTimeout = 90 * time.Second

var (
	// lock provider block: provider "registry.../aws" { ... version = "x.y.z"
	lockProviderHeaderRe = regexp.MustCompile(`(?m)^provider\s+"([^"]+)"\s*\{`)
	lockVersionRe        = regexp.MustCompile(`(?m)^\s*version\s*=\s*"([^"]+)"`)
)

// runPostInitStage 正式 stage：init 段落后执行。失败只 warn，永不 fail task。
// initActuallyRan 仅用于日志；capture 决策以 provider 版本 stamp 为准。
func (s *TerraformExecutor) runPostInitStage(
	ctx context.Context,
	workDir string,
	task *models.WorkspaceTask,
	workspace *models.Workspace,
	logger *TerraformLogger,
	initActuallyRan bool,
) {
	logger.StageBegin("post_init")
	defer logger.StageEnd("post_init")

	if initActuallyRan {
		logger.Info("[post_init] init ran this cycle; evaluating provider schema cache")
	} else {
		logger.Info("[post_init] init skipped; evaluating provider schema cache by version stamp")
	}

	if err := s.captureManifestProviderSchemaIfNeeded(ctx, workDir, task, workspace, logger); err != nil {
		logger.Warn("[post_init] schema capture skipped/failed (non-fatal): %v", err)
		return
	}
}

func (s *TerraformExecutor) captureManifestProviderSchemaIfNeeded(
	ctx context.Context,
	workDir string,
	task *models.WorkspaceTask,
	workspace *models.Workspace,
	logger *TerraformLogger,
) error {
	if workspace == nil {
		return fmt.Errorf("workspace is nil")
	}

	// 是否 manifest-managed 必须以 DB 为准，不能看 workspace.ManifestDeploymentID。
	// ExecutePlan 对 Manifest [Run]（ExternalFiles）会在栈上清空 deployment/tag，
	// 以便 GenerateConfigFiles 走草稿落盘；DB 行仍是 manifest 托管（资源页横幅同源）。
	// schema 按 (manifest_id, subpath) 落库，与 Run/正式部署无关。
	meta, metaErr := s.dataAccessor.GetManifestProviderSchemaMetaByWorkspace(workspace.WorkspaceID)
	if metaErr != nil {
		if isNotManifestManagedErr(metaErr) {
			logger.Info("[post_init] workspace not manifest-managed in DB; skip schema capture")
			return nil
		}
		// 其它错误：继续尝试 capture+upsert（upsert 会再解析键）；meta 视为无缓存
		logger.Warn("[post_init] meta lookup failed (will try capture): %v", metaErr)
		meta = nil
	}

	runDir := s.ResolveRunDir(workspace, workDir)
	lockPath := filepath.Join(runDir, ".terraform.lock.hcl")
	providers, versionsKey, err := parseProviderVersionsFromLockFile(lockPath)
	if err != nil {
		return fmt.Errorf("parse lock providers: %w", err)
	}
	if versionsKey == "" || len(providers) == 0 {
		logger.Info("[post_init] no providers in lock; skip")
		return nil
	}
	logger.Info("[post_init] lock providers: %s (key=%s)", formatProviderVersions(providers), versionsKey[:min(12, len(versionsKey))])

	// 版本未变 → 不跑 providers schema、不写库
	if meta != nil && meta.ProviderVersionsKey != "" &&
		meta.ProviderVersionsKey == versionsKey &&
		meta.SchemaKind == models.ManifestProviderSchemaKindTypes {
		logger.Info("[post_init] provider versions unchanged (key match); skip schema extract & upsert")
		return nil
	}
	if meta != nil && meta.ProviderVersionsKey != "" {
		logger.Info("[post_init] provider versions changed; re-capturing (manifest=%s subpath=%q)",
			meta.ManifestID, meta.Subpath)
	} else if meta != nil {
		logger.Info("[post_init] no cached schema yet (manifest=%s subpath=%q); capturing",
			meta.ManifestID, meta.Subpath)
	} else {
		logger.Info("[post_init] no cached schema for this workspace's manifest+subpath; capturing")
	}

	// 需要提取：要求 .terraform 存在
	if _, err := os.Stat(filepath.Join(runDir, ".terraform")); err != nil {
		return fmt.Errorf(".terraform missing under runDir %s: %w", runDir, err)
	}

	tfBinary, err := s.resolveTerraformBinary(workspace)
	if err != nil {
		return err
	}

	schemaCtx, cancel := context.WithTimeout(ctx, postInitSchemaTimeout)
	defer cancel()

	resources, dataSources, err := extractProviderTypeNames(schemaCtx, tfBinary, runDir, s.buildEnvironmentVariables(workspace))
	if err != nil {
		return fmt.Errorf("providers schema: %w", err)
	}

	providersJSON, _ := json.Marshal(providers)
	resourcesJSON, _ := json.Marshal(resources)
	dataJSON, _ := json.Marshal(dataSources)
	contentHash := shortHash(string(resourcesJSON) + "|" + string(dataJSON) + "|" + versionsKey)

	row := &models.ManifestProviderSchema{
		SchemaKind:          models.ManifestProviderSchemaKindTypes,
		Providers:           providersJSON,
		ProviderVersionsKey: versionsKey,
		Resources:           resourcesJSON,
		DataSources:         dataJSON,
		ContentHash:         contentHash,
		TerraformVersion:    workspace.TerraformVersion,
		SourceWorkspaceID:   workspace.WorkspaceID,
		SourceTaskID:        &task.ID,
		CapturedAt:          time.Now().UTC(),
	}

	if err := s.dataAccessor.UpsertManifestProviderSchemaByWorkspace(workspace.WorkspaceID, row); err != nil {
		return fmt.Errorf("upsert schema: %w", err)
	}

	logger.Info("[post_init] ✓ saved provider schema types: resources=%d data=%d hash=%s",
		len(resources), len(dataSources), contentHash)
	return nil
}

func (s *TerraformExecutor) resolveTerraformBinary(workspace *models.Workspace) (string, error) {
	if s.downloader == nil {
		return "", fmt.Errorf("terraform downloader not initialized")
	}
	path, err := s.downloader.EnsureTerraformBinary(workspace.TerraformVersion)
	if err != nil {
		return "", fmt.Errorf("ensure terraform binary: %w", err)
	}
	return path, nil
}

// parseProviderVersionsFromLockFile 从 .terraform.lock.hcl 提取 provider 版本（廉价，无 CLI）。
func parseProviderVersionsFromLockFile(lockPath string) ([]models.ProviderVersionRef, string, error) {
	data, err := os.ReadFile(lockPath)
	if err != nil {
		return nil, "", err
	}
	return parseProviderVersionsFromLock(string(data))
}

func parseProviderVersionsFromLock(content string) ([]models.ProviderVersionRef, string, error) {
	// 按 provider 块切分
	idxs := lockProviderHeaderRe.FindAllStringSubmatchIndex(content, -1)
	if len(idxs) == 0 {
		return nil, "", nil
	}
	var refs []models.ProviderVersionRef
	for i, loc := range idxs {
		source := content[loc[2]:loc[3]]
		start := loc[1]
		end := len(content)
		if i+1 < len(idxs) {
			end = idxs[i+1][0]
		}
		block := content[start:end]
		vm := lockVersionRe.FindStringSubmatch(block)
		if vm == nil {
			continue
		}
		refs = append(refs, models.ProviderVersionRef{
			Source:  source,
			Version: vm[1],
		})
	}
	sort.Slice(refs, func(i, j int) bool {
		if refs[i].Source == refs[j].Source {
			return refs[i].Version < refs[j].Version
		}
		return refs[i].Source < refs[j].Source
	})
	return refs, providerVersionsKey(refs), nil
}

func providerVersionsKey(refs []models.ProviderVersionRef) string {
	if len(refs) == 0 {
		return ""
	}
	parts := make([]string, 0, len(refs))
	for _, r := range refs {
		parts = append(parts, r.Source+"@"+r.Version)
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return hex.EncodeToString(sum[:])
}

func formatProviderVersions(refs []models.ProviderVersionRef) string {
	parts := make([]string, 0, len(refs))
	for _, r := range refs {
		// 缩短 registry 前缀便于日志
		src := r.Source
		if i := strings.LastIndex(src, "/"); i >= 0 {
			src = src[i+1:]
		}
		parts = append(parts, src+"@"+r.Version)
	}
	return strings.Join(parts, ", ")
}

func extractProviderTypeNames(ctx context.Context, tfBinary, runDir string, env []string) (resources, data []string, err error) {
	cmd := exec.CommandContext(ctx, tfBinary, "providers", "schema", "-json")
	cmd.Dir = runDir
	cmd.Env = env
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return nil, nil, fmt.Errorf("%w: %s", err, string(ee.Stderr))
		}
		return nil, nil, err
	}

	var raw struct {
		ProviderSchemas map[string]struct {
			ResourceSchemas   map[string]json.RawMessage `json:"resource_schemas"`
			DataSourceSchemas map[string]json.RawMessage `json:"data_source_schemas"`
		} `json:"provider_schemas"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, nil, fmt.Errorf("parse schema json: %w", err)
	}

	resSet := map[string]struct{}{}
	dataSet := map[string]struct{}{}
	for _, ps := range raw.ProviderSchemas {
		for name := range ps.ResourceSchemas {
			resSet[name] = struct{}{}
		}
		for name := range ps.DataSourceSchemas {
			dataSet[name] = struct{}{}
		}
	}
	resources = sortedKeys(resSet)
	data = sortedKeys(dataSet)
	return resources, data, nil
}

func sortedKeys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func shortHash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:8])
}

func newManifestProviderSchemaID() string {
	var b [10]byte
	_, _ = rand.Read(b[:])
	return "mps-" + hex.EncodeToString(b[:])
}

// isNotManifestManagedErr 识别 resolveManifestSubpathKey 返回的「非托管」错误
func isNotManifestManagedErr(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "not manifest-managed")
}
