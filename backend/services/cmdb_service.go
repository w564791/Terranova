package services

import (
	"context"
	"encoding/json"
	"fmt"
	"iac-platform/internal/models"
	"log"
	"regexp"
	"strings"
	"time"

	"gorm.io/gorm"
)

// CMDBService CMDB服务
type CMDBService struct {
	db            *gorm.DB
	nameExtractor *ResourceNameExtractor
}

// NewCMDBService 创建CMDB服务
func NewCMDBService(db *gorm.DB) *CMDBService {
	return &CMDBService{
		db:            db,
		nameExtractor: NewResourceNameExtractor(),
	}
}

// ResourceNameExtractor 资源名称提取器
type ResourceNameExtractor struct {
	fallbackRules map[string][]string
}

// NewResourceNameExtractor 创建资源名称提取器
func NewResourceNameExtractor() *ResourceNameExtractor {
	return &ResourceNameExtractor{
		fallbackRules: map[string][]string{
			// EC2相关
			"aws_instance":          {"private_dns", "private_ip"},
			"aws_launch_template":   {"name_prefix"},
			"aws_autoscaling_group": {"name_prefix"},

			// 网络相关
			"aws_vpc":             {"cidr_block"},
			"aws_subnet":          {"cidr_block", "availability_zone"},
			"aws_security_group":  {"name_prefix"},
			"aws_lb":              {"dns_name"},
			"aws_lb_target_group": {"name_prefix"},

			// 数据库相关
			"aws_db_instance": {"db_instance_identifier", "endpoint"},
			"aws_rds_cluster": {"cluster_identifier", "endpoint"},

			// 存储相关
			"aws_s3_bucket":  {"bucket"},
			"aws_ebs_volume": {"availability_zone"},

			// IAM相关
			"aws_iam_role":             {"name_prefix"},
			"aws_iam_policy":           {"name_prefix"},
			"aws_iam_instance_profile": {"name_prefix"},

			// EKS相关
			"aws_eks_cluster":    {"endpoint"},
			"aws_eks_node_group": {"node_group_name"},
		},
	}
}

// ExtractName 从资源属性中提取名称
func (e *ResourceNameExtractor) ExtractName(resourceType string, attributes map[string]interface{}) string {
	// 1. 优先提取name字段
	if name := cmdbGetString(attributes, "name"); name != "" {
		return name
	}

	// 2. 尝试从tags中提取Name
	if tags := cmdbGetMap(attributes, "tags"); tags != nil {
		if name := cmdbGetString(tags, "Name"); name != "" {
			return name
		}
	}
	if tagsAll := cmdbGetMap(attributes, "tags_all"); tagsAll != nil {
		if name := cmdbGetString(tagsAll, "Name"); name != "" {
			return name
		}
	}

	// 3. 尝试description字段
	if desc := cmdbGetString(attributes, "description"); desc != "" {
		if len(desc) > 50 {
			return desc[:50] + "..."
		}
		return desc
	}

	// 4. 资源类型特定的fallback
	if fields, ok := e.fallbackRules[resourceType]; ok {
		for _, field := range fields {
			if value := cmdbGetString(attributes, field); value != "" {
				return value
			}
		}
	}

	// 5. 最终使用ID
	if id := cmdbGetString(attributes, "id"); id != "" {
		return id
	}

	return "unnamed"
}

// cmdbGetString 从map中获取字符串
func cmdbGetString(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// cmdbGetMap 从map中获取子map
func cmdbGetMap(m map[string]interface{}, key string) map[string]interface{} {
	if v, ok := m[key]; ok {
		if sub, ok := v.(map[string]interface{}); ok {
			return sub
		}
	}
	return nil
}

// SyncWorkspaceResources 同步workspace的资源索引
func (s *CMDBService) SyncWorkspaceResources(workspaceID string, triggeredBy ...string) error {
	triggerSource := "manual"
	if len(triggeredBy) > 0 && triggeredBy[0] != "" {
		triggerSource = triggeredBy[0]
	}

	// 1. 获取最新的state版本
	var stateVersion models.WorkspaceStateVersion
	if err := s.db.Where("workspace_id = ?", workspaceID).
		Order("version DESC").
		First(&stateVersion).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil // 没有state，跳过
		}
		return fmt.Errorf("failed to get state version: %w", err)
	}

	// 获取 workspace 名称
	var workspace models.Workspace
	s.db.Select("name").Where("workspace_id = ?", workspaceID).First(&workspace)

	// 创建 sync log
	syncLog := models.CMDBSyncLog{
		SourceID:    workspaceID,
		SourceType:  "workspace",
		SourceName:  workspace.Name,
		TriggeredBy: triggerSource,
		StartedAt:   time.Now(),
		Status:      models.SyncStatusRunning,
	}
	s.db.Create(&syncLog)

	// 2. Content已经是JSONB类型（map[string]interface{}），直接使用
	stateContent := map[string]interface{}(stateVersion.Content)

	// 3. 解析并同步资源
	counts, err := s.parseAndSyncState(workspaceID, stateContent, stateVersion.ID)
	if err != nil {
		now := time.Now()
		s.db.Model(&syncLog).Updates(map[string]interface{}{
			"status":        models.SyncStatusFailed,
			"completed_at":  now,
			"error_message": err.Error(),
		})
		return err
	}

	// 更新 sync log
	now := time.Now()
	s.db.Model(&syncLog).Updates(map[string]interface{}{
		"status":           models.SyncStatusSuccess,
		"completed_at":     now,
		"resources_synced": counts.Added + counts.Updated,
		"resources_added":  counts.Added,
		"resources_updated": counts.Updated,
		"resources_deleted": counts.Deleted,
	})

	// 4. 异步生成资源摘要 → 完成后触发 embedding
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		// 4a. 资源摘要（同步执行，在此 goroutine 内阻塞）
		summaryService := NewResourceSummaryService(s.db)
		if err := summaryService.GenerateSummaries(ctx, workspaceID); err != nil {
			log.Printf("[CMDB] Resource summary failed for workspace %s: %v", workspaceID, err)
		}

		// 4b. 摘要完成后触发 embedding（包含 resource_summary 的增强内容）
		log.Printf("[CMDB] Starting embedding sync for workspace %s", workspaceID)
		embeddingWorker := NewEmbeddingWorker(s.db)
		if err := embeddingWorker.SyncWorkspace(workspaceID); err != nil {
			log.Printf("[CMDB] Embedding sync failed for workspace %s: %v", workspaceID, err)
		} else {
			log.Printf("[CMDB] Embedding sync completed for workspace %s", workspaceID)
		}
	}()

	return nil
}

// SyncCounts 同步资源计数
type SyncCounts struct {
	Added   int
	Updated int
	Deleted int
}

// parseAndSyncState 解析State并同步到资源索引
// 使用增量更新模式，保留已有的 embedding 数据
func (s *CMDBService) parseAndSyncState(workspaceID string, stateContent map[string]interface{}, stateVersionID uint) (*SyncCounts, error) {
	// 1. 解析resources数组
	resources, ok := stateContent["resources"].([]interface{})
	if !ok {
		return &SyncCounts{}, nil // 空state
	}

	// 2. 解析每个资源
	var indexRecords []models.ResourceIndex
	moduleSet := make(map[string]bool)

	for _, res := range resources {
		resMap, ok := res.(map[string]interface{})
		if !ok {
			continue
		}

		records := s.parseResource(workspaceID, resMap, stateVersionID)
		indexRecords = append(indexRecords, records...)

		// 收集module路径
		for _, record := range records {
			if record.ModulePath != "" {
				s.collectModulePaths(record.ModulePath, moduleSet)
			}
		}
	}

	// 3. 事务更新数据库 - 使用增量更新模式
	counts := &SyncCounts{}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		// 获取现有资源（用于增量更新）
		existingResources := make(map[string]*models.ResourceIndex)
		var existing []models.ResourceIndex
		if err := tx.Where("workspace_id = ? AND source_type = ?", workspaceID, "terraform").Find(&existing).Error; err != nil {
			return err
		}
		for i := range existing {
			existingResources[existing[i].TerraformAddress] = &existing[i]
		}

		// 处理每个资源
		processedAddresses := make(map[string]bool)
		for _, record := range indexRecords {
			processedAddresses[record.TerraformAddress] = true

			if existingRecord, exists := existingResources[record.TerraformAddress]; exists {
				// 更新现有记录 - 使用 Updates 而不是 Save，避免覆盖 embedding 相关字段
				if err := tx.Model(&models.ResourceIndex{}).
					Where("id = ?", existingRecord.ID).
					Updates(map[string]interface{}{
						"resource_type":       record.ResourceType,
						"resource_name":       record.ResourceName,
						"resource_mode":       record.ResourceMode,
						"index_key":           record.IndexKey,
						"cloud_resource_id":   record.CloudResourceID,
						"cloud_resource_name": record.CloudResourceName,
						"cloud_resource_arn":  record.CloudResourceARN,
						"description":         record.Description,
						"module_path":         record.ModulePath,
						"module_depth":        record.ModuleDepth,
						"parent_module_path":  record.ParentModulePath,
						"root_module_name":    record.RootModuleName,
						"attributes":          record.Attributes,
						"tags":                record.Tags,
						"provider":            record.Provider,
						"state_version_id":    record.StateVersionID,
						"last_synced_at":      record.LastSyncedAt,
						// 注意：不更新 embedding, embedding_text, embedding_model, embedding_updated_at 字段
						// 这样可以保留已生成的 embedding 数据
					}).Error; err != nil {
					return err
				}
				counts.Updated++
			} else {
				// 创建新记录
				if err := tx.Create(&record).Error; err != nil {
					return err
				}
				counts.Added++
			}
		}

		// 删除不再存在的资源
		for address, existingRecord := range existingResources {
			if !processedAddresses[address] {
				if err := tx.Delete(existingRecord).Error; err != nil {
					return err
				}
				counts.Deleted++
			}
		}

		// 更新module层级表
		return s.syncModuleHierarchy(tx, workspaceID, moduleSet)
	})
	return counts, err
}

// parseResource 解析单个资源
func (s *CMDBService) parseResource(workspaceID string, resMap map[string]interface{}, stateVersionID uint) []models.ResourceIndex {
	var records []models.ResourceIndex

	mode := cmdbGetString(resMap, "mode")
	resourceType := cmdbGetString(resMap, "type")
	resourceName := cmdbGetString(resMap, "name")
	modulePath := cmdbGetString(resMap, "module")
	provider := cmdbGetString(resMap, "provider")

	instances, ok := resMap["instances"].([]interface{})
	if !ok || len(instances) == 0 {
		return records
	}

	for _, inst := range instances {
		instMap, ok := inst.(map[string]interface{})
		if !ok {
			continue
		}

		attributes, _ := instMap["attributes"].(map[string]interface{})
		indexKey := s.getIndexKey(instMap)

		// 构建Terraform地址
		address := s.buildTerraformAddress(modulePath, resourceType, resourceName, indexKey)

		// 提取云资源信息
		cloudID := cmdbGetString(attributes, "id")
		cloudName := s.nameExtractor.ExtractName(resourceType, attributes)
		cloudARN := cmdbGetString(attributes, "arn")
		description := cmdbGetString(attributes, "description")

		// 提取tags
		var tagsJSON json.RawMessage
		if tags := cmdbGetMap(attributes, "tags"); tags != nil {
			tagsJSON, _ = json.Marshal(tags)
		}

		// 提取attributes（可选，用于详情展示）
		var attrsJSON json.RawMessage
		if attributes != nil {
			attrsJSON, _ = json.Marshal(attributes)
		}

		// 解析module层级
		moduleDepth, parentPath, rootModule := s.parseModulePath(modulePath)

		record := models.ResourceIndex{
			WorkspaceID:       workspaceID,
			TerraformAddress:  address,
			ResourceType:      resourceType,
			ResourceName:      resourceName,
			ResourceMode:      mode,
			IndexKey:          indexKey,
			CloudResourceID:   cloudID,
			CloudResourceName: cloudName,
			CloudResourceARN:  cloudARN,
			Description:       description,
			ModulePath:        modulePath,
			ModuleDepth:       moduleDepth,
			ParentModulePath:  parentPath,
			RootModuleName:    rootModule,
			Attributes:        attrsJSON,
			Tags:              tagsJSON,
			Provider:          provider,
			StateVersionID:    &stateVersionID,
			LastSyncedAt:      time.Now(),
		}

		records = append(records, record)
	}

	return records
}

// getIndexKey 获取资源的index key
func (s *CMDBService) getIndexKey(instMap map[string]interface{}) string {
	if indexKey, ok := instMap["index_key"]; ok {
		switch v := indexKey.(type) {
		case string:
			return fmt.Sprintf(`"%s"`, v)
		case float64:
			return fmt.Sprintf("%d", int(v))
		case int:
			return fmt.Sprintf("%d", v)
		}
	}
	return ""
}

// buildTerraformAddress 构建完整的Terraform地址
func (s *CMDBService) buildTerraformAddress(modulePath, resourceType, resourceName, indexKey string) string {
	var parts []string

	if modulePath != "" {
		parts = append(parts, modulePath)
	}

	parts = append(parts, fmt.Sprintf("%s.%s", resourceType, resourceName))

	address := strings.Join(parts, ".")

	if indexKey != "" {
		address = fmt.Sprintf("%s[%s]", address, indexKey)
	}

	return address
}

// parseModulePath 解析module路径
func (s *CMDBService) parseModulePath(modulePath string) (depth int, parentPath, rootModule string) {
	if modulePath == "" {
		return 0, "", ""
	}

	// 解析 module.xxx.module.yyy.module.zzz 格式
	parts := strings.Split(modulePath, ".module.")
	depth = len(parts)

	if depth > 1 {
		parentPath = strings.Join(parts[:depth-1], ".module.")
		if !strings.HasPrefix(parentPath, "module.") {
			parentPath = "module." + parentPath
		}
	}

	// 提取根module名称
	if strings.HasPrefix(modulePath, "module.") {
		firstPart := strings.Split(modulePath[7:], ".")[0]
		// 移除for_each的key
		if idx := strings.Index(firstPart, "["); idx > 0 {
			firstPart = firstPart[:idx]
		}
		rootModule = firstPart
	}

	return
}

// collectModulePaths 收集所有module路径
func (s *CMDBService) collectModulePaths(modulePath string, moduleSet map[string]bool) {
	if modulePath == "" {
		return
	}

	// 添加当前路径
	moduleSet[modulePath] = true

	// 递归添加父路径
	parts := strings.Split(modulePath, ".module.")
	for i := len(parts) - 1; i > 0; i-- {
		parentPath := strings.Join(parts[:i], ".module.")
		if !strings.HasPrefix(parentPath, "module.") {
			parentPath = "module." + parentPath
		}
		moduleSet[parentPath] = true
	}
}

// syncModuleHierarchy 同步module层级表
func (s *CMDBService) syncModuleHierarchy(tx *gorm.DB, workspaceID string, moduleSet map[string]bool) error {
	// 删除旧记录
	if err := tx.Where("workspace_id = ?", workspaceID).Delete(&models.ModuleHierarchy{}).Error; err != nil {
		return err
	}

	if len(moduleSet) == 0 {
		return nil
	}

	// 构建module层级记录
	var modules []models.ModuleHierarchy
	for modulePath := range moduleSet {
		depth, parentPath, _ := s.parseModulePath(modulePath)
		moduleName, moduleKey := s.extractModuleNameAndKey(modulePath)

		// 统计资源数
		var resourceCount int64
		tx.Model(&models.ResourceIndex{}).
			Where("workspace_id = ? AND module_path = ?", workspaceID, modulePath).
			Count(&resourceCount)

		// 统计子module数
		var childCount int64
		tx.Model(&models.ModuleHierarchy{}).
			Where("workspace_id = ? AND parent_path = ?", workspaceID, modulePath).
			Count(&childCount)

		module := models.ModuleHierarchy{
			WorkspaceID:        workspaceID,
			ModulePath:         modulePath,
			ModuleName:         moduleName,
			ModuleKey:          moduleKey,
			ParentPath:         parentPath,
			Depth:              depth,
			ResourceCount:      int(resourceCount),
			TotalResourceCount: int(resourceCount), // 简化处理，后续可优化
			ChildModuleCount:   int(childCount),
			LastSyncedAt:       time.Now(),
		}
		modules = append(modules, module)
	}

	if len(modules) > 0 {
		return tx.CreateInBatches(modules, 100).Error
	}

	return nil
}

// extractModuleNameAndKey 从module路径提取名称和key
func (s *CMDBService) extractModuleNameAndKey(modulePath string) (name, key string) {
	// 获取最后一个module部分
	parts := strings.Split(modulePath, ".module.")
	lastPart := parts[len(parts)-1]

	// 移除开头的"module."（如果有）
	if strings.HasPrefix(lastPart, "module.") {
		lastPart = lastPart[7:]
	}

	// 检查是否有for_each key
	re := regexp.MustCompile(`^([^\[]+)(?:\["([^"]+)"\])?$`)
	matches := re.FindStringSubmatch(lastPart)
	if len(matches) >= 2 {
		name = matches[1]
		if len(matches) >= 3 {
			key = matches[2]
		}
	} else {
		name = lastPart
	}

	return
}

// platformResourceJoinSQL 将 resource_index 关联到 workspace_resources。
// 覆盖：
//  1. 精确 terraform_address / 去索引后的 address（manifest 原生资源）
//  2. module 路径 / module.name（manifest module 块）
//  3. root_module_name 精确匹配（manifest 根 module 名）
//  4. 平台 module 命名：Provider_type_resourceName 后缀匹配
const platformResourceJoinSQL = `
	ri.workspace_id = wr.workspace_id
	AND ri.source_type = 'terraform'
	AND (
		wr.resource_id = ri.terraform_address
		OR wr.resource_id = split_part(ri.terraform_address, '[', 1)
		OR (wr.resource_type = 'module' AND ri.module_path <> '' AND wr.resource_id = ri.module_path)
		OR (wr.resource_type = 'module' AND ri.root_module_name <> '' AND wr.resource_id = 'module.' || ri.root_module_name)
		OR (wr.resource_type = 'module' AND ri.root_module_name <> '' AND wr.resource_name = ri.root_module_name)
		OR (ri.root_module_name <> '' AND ri.root_module_name LIKE '%\_' || wr.resource_name)
	)
`

// jumpURLSelectSQL 生成 CMDB 跳转链接：
// - manifest workspace → manifest 编辑器（带 resource 深链，优先 terraform address）
// - 普通 module workspace → /workspaces/.../resources/{id}
// - 外部数据源 → NULL
const jumpURLSelectSQL = `
	CASE
		WHEN ri.source_type = 'external' THEN NULL
		WHEN m.id IS NOT NULL THEN
			CONCAT(
				'/admin/manifests-v2/', m.id, '/edit?org=', m.organization_id::text,
				'&resource=', COALESCE(
					NULLIF(split_part(ri.terraform_address, '[', 1), ''),
					NULLIF(wr.resource_id, ''),
					ri.terraform_address
				),
				CASE WHEN COALESCE(w.manifest_subpath, '') <> '' THEN CONCAT('&subpath=', w.manifest_subpath) ELSE '' END,
				CASE WHEN COALESCE(md.version_id, '') <> '' THEN CONCAT('&version=', md.version_id) ELSE '' END
			)
		WHEN wr.id IS NOT NULL THEN CONCAT('/workspaces/', ri.workspace_id, '/resources/', wr.id)
		ELSE NULL
	END
`

// SearchResources 搜索资源
func (s *CMDBService) SearchResources(query string, workspaceID string, resourceType string, limit int) ([]models.ResourceSearchResult, error) {
	if limit <= 0 {
		limit = 20
	}

	var results []models.ResourceSearchResult

	// 构建查询 - 使用多策略匹配关联 workspace_resources；manifest workspace 跳编辑器
	db := s.db.Table("resource_index ri").
		Select(fmt.Sprintf(`
			ri.workspace_id,
			w.name as workspace_name,
			ri.terraform_address,
			ri.resource_type,
			ri.resource_name,
			ri.cloud_resource_id,
			ri.cloud_resource_name,
			ri.cloud_resource_arn,
			ri.description,
			ri.module_path,
			ri.root_module_name,
			ri.source_type,
			ri.external_source_id,
			es.name as external_source_name,
			ri.cloud_provider,
			ri.cloud_account_id,
			ri.cloud_account_name,
			ri.cloud_region,
			ri.resource_summary,
			wr.id as platform_resource_id,
			wr.resource_name as platform_resource_name,
			%s as jump_url,
			CASE
				WHEN wr.id IS NOT NULL AND wr.is_active = false THEN true
				ELSE false
			END as is_resource_deleted,
			CASE 
				WHEN ri.cloud_resource_id = ? THEN 1.0
				WHEN ri.cloud_resource_name = ? THEN 0.9
				WHEN ri.cloud_resource_arn = ? THEN 0.85
				WHEN ri.cloud_resource_id LIKE ? THEN 0.8
				WHEN ri.cloud_resource_name LIKE ? THEN 0.7
				WHEN ri.cloud_resource_arn LIKE ? THEN 0.65
				WHEN ri.cloud_resource_id LIKE ? THEN 0.6
				WHEN ri.cloud_resource_name LIKE ? THEN 0.5
				WHEN ri.cloud_resource_arn LIKE ? THEN 0.45
				WHEN ri.description LIKE ? THEN 0.4
				WHEN ri.terraform_address LIKE ? THEN 0.3
				ELSE 0.1
			END as match_rank
		`, jumpURLSelectSQL), query, query, query, query+"%", query+"%", query+"%", "%"+query+"%", "%"+query+"%", "%"+query+"%", "%"+query+"%", "%"+query+"%").
		Joins("LEFT JOIN workspaces w ON ri.workspace_id = w.workspace_id").
		Joins("LEFT JOIN workspace_resources wr ON " + platformResourceJoinSQL).
		Joins("LEFT JOIN manifest_deployments md ON md.id = COALESCE(wr.manifest_deployment_id, w.manifest_deployment_id)").
		Joins("LEFT JOIN manifests m ON m.id = md.manifest_id").
		Joins("LEFT JOIN cmdb_external_sources es ON ri.external_source_id = es.source_id").
		Where("ri.resource_mode = ?", "managed").
		Where(`
			ri.cloud_resource_id ILIKE ? OR
			ri.cloud_resource_name ILIKE ? OR
			ri.cloud_resource_arn ILIKE ? OR
			ri.description ILIKE ? OR
			ri.terraform_address ILIKE ?
		`, "%"+query+"%", "%"+query+"%", "%"+query+"%", "%"+query+"%", "%"+query+"%")

	if workspaceID != "" {
		db = db.Where("ri.workspace_id = ?", workspaceID)
	}

	if resourceType != "" {
		db = db.Where("ri.resource_type = ?", resourceType)
	}

	// 排序：内部数据（terraform）优先，同一 ri 下未删除的 wr 优先，然后按匹配度排序
	// 不加 LIMIT，在 Go 层去重后再截断（JOIN 可能产生同 ri.id 的多行）
	var rawResults []models.ResourceSearchResult
	if err := db.Order("ri.source_type ASC, is_resource_deleted ASC, match_rank DESC, ri.cloud_resource_name").
		Scan(&rawResults).Error; err != nil {
		return nil, err
	}

	// 按 workspace_id + terraform_address 去重（等价于 ri.id 唯一性）
	seen := make(map[string]bool)
	for _, r := range rawResults {
		key := r.WorkspaceID + "|" + r.TerraformAddress
		if seen[key] {
			continue
		}
		seen[key] = true
		results = append(results, r)
		if len(results) >= limit {
			break
		}
	}

	return results, nil
}

// manifestJumpInfo 用于构建 manifest 编辑器跳转链接
type manifestJumpInfo struct {
	ManifestID string
	OrgID      int
	VersionID  string
	Subpath    string
}

// loadManifestJumpInfo 若 workspace 绑定了 manifest deployment，返回跳转所需字段
func (s *CMDBService) loadManifestJumpInfo(workspace *models.Workspace) *manifestJumpInfo {
	if workspace == nil || workspace.ManifestDeploymentID == nil || *workspace.ManifestDeploymentID == "" {
		return nil
	}
	type row struct {
		ManifestID string
		OrgID      int
		VersionID  string
	}
	var r row
	err := s.db.Table("manifest_deployments md").
		Select("md.manifest_id, m.organization_id as org_id, md.version_id").
		Joins("JOIN manifests m ON m.id = md.manifest_id").
		Where("md.id = ?", *workspace.ManifestDeploymentID).
		Take(&r).Error
	if err != nil || r.ManifestID == "" {
		return nil
	}
	info := &manifestJumpInfo{
		ManifestID: r.ManifestID,
		OrgID:      r.OrgID,
		VersionID:  r.VersionID,
	}
	if workspace.ManifestSubpath != nil {
		info.Subpath = *workspace.ManifestSubpath
	}
	return info
}

// buildResourceJumpURL 生成资源跳转 URL（manifest 编辑器或平台资源详情）
// terraformAddress 优先用于深链定位；无 address 时回退到 platform resource_id。
func buildResourceJumpURL(workspaceID string, pr *models.WorkspaceResource, terraformAddress string, mInfo *manifestJumpInfo) string {
	resourceRef := terraformAddress
	if idx := strings.Index(resourceRef, "["); idx > 0 {
		resourceRef = resourceRef[:idx]
	}
	if resourceRef == "" && pr != nil {
		resourceRef = pr.ResourceID
	}

	// Manifest 管理：跳编辑器并定位到对应 HCL 块
	if mInfo != nil && mInfo.ManifestID != "" {
		if resourceRef == "" {
			resourceRef = "module"
		}
		url := fmt.Sprintf("/admin/manifests-v2/%s/edit?org=%d&resource=%s", mInfo.ManifestID, mInfo.OrgID, resourceRef)
		if mInfo.Subpath != "" {
			url += "&subpath=" + mInfo.Subpath
		}
		if mInfo.VersionID != "" {
			url += "&version=" + mInfo.VersionID
		}
		return url
	}

	if pr != nil {
		return fmt.Sprintf("/workspaces/%s/resources/%d", workspaceID, pr.ID)
	}
	return ""
}

// GetWorkspaceResourceTree 获取workspace的资源树
func (s *CMDBService) GetWorkspaceResourceTree(workspaceID string) (*models.WorkspaceResourceTree, error) {
	// 获取workspace信息
	var workspace models.Workspace
	if err := s.db.Where("workspace_id = ?", workspaceID).First(&workspace).Error; err != nil {
		return nil, fmt.Errorf("workspace not found: %w", err)
	}

	// 获取所有managed资源（排除data sources和外部数据源）
	var resources []models.ResourceIndex
	if err := s.db.Where("workspace_id = ? AND resource_mode = ? AND source_type = ?", workspaceID, "managed", "terraform").
		Order("module_path, terraform_address").
		Find(&resources).Error; err != nil {
		return nil, err
	}

	// 获取所有module
	var modules []models.ModuleHierarchy
	if err := s.db.Where("workspace_id = ?", workspaceID).
		Order("depth, module_path").
		Find(&modules).Error; err != nil {
		return nil, err
	}

	// 获取平台资源列表（按 name / resource_id 多 key 索引）
	var wsResources []models.WorkspaceResource
	_ = s.db.Where("workspace_id = ? AND is_active = true", workspaceID).Find(&wsResources).Error

	// 构建树结构
	tree := s.buildResourceTree(modules, resources, wsResources, workspaceID, &workspace)

	return &models.WorkspaceResourceTree{
		WorkspaceID:    workspaceID,
		WorkspaceName:  workspace.Name,
		TotalResources: len(resources),
		Tree:           tree,
	}, nil
}

// buildResourceTree 构建资源树
func (s *CMDBService) buildResourceTree(
	modules []models.ModuleHierarchy,
	resources []models.ResourceIndex,
	wsResources []models.WorkspaceResource,
	workspaceID string,
	workspace *models.Workspace,
) []*models.ResourceTreeNode {
	mInfo := s.loadManifestJumpInfo(workspace)

	// 多 key 索引：resource_name / resource_id / type.name
	byName := make(map[string]*models.WorkspaceResource)
	byResourceID := make(map[string]*models.WorkspaceResource)
	for i := range wsResources {
		pr := &wsResources[i]
		byName[pr.ResourceName] = pr
		if pr.ResourceID != "" {
			byResourceID[pr.ResourceID] = pr
		}
	}

	// 查找平台资源：支持 module 名、resource_id、type.name、后缀匹配
	findPlatformResource := func(moduleName, resourceID, resourceType, resourceName string) *models.WorkspaceResource {
		if resourceID != "" {
			if pr, ok := byResourceID[resourceID]; ok {
				return pr
			}
			// 去索引：aws_s3_bucket.this[0] → aws_s3_bucket.this
			if idx := strings.Index(resourceID, "["); idx > 0 {
				if pr, ok := byResourceID[resourceID[:idx]]; ok {
					return pr
				}
			}
		}
		if resourceType != "" && resourceName != "" {
			key := resourceType + "." + resourceName
			if pr, ok := byResourceID[key]; ok {
				return pr
			}
		}
		if moduleName != "" {
			if pr, ok := byName[moduleName]; ok {
				return pr
			}
			if pr, ok := byResourceID["module."+moduleName]; ok {
				return pr
			}
			// 后缀匹配：平台 module 命名 Provider_type_resourceName
			var bestMatch *models.WorkspaceResource
			bestLen := 0
			for resName, pr := range byName {
				suffix := "_" + resName
				if strings.HasSuffix(moduleName, suffix) && len(resName) > bestLen {
					bestMatch = pr
					bestLen = len(resName)
				}
			}
			if bestMatch != nil {
				return bestMatch
			}
		}
		return nil
	}

	// 创建module节点映射
	moduleNodes := make(map[string]*models.ResourceTreeNode)

	for _, m := range modules {
		node := &models.ResourceTreeNode{
			Type:          "module",
			Name:          m.ModuleName,
			Path:          m.ModulePath,
			ResourceCount: m.TotalResourceCount,
			Children:      []*models.ResourceTreeNode{},
		}

		// 为根module添加跳转链接
		if m.ParentPath == "" {
			if pr := findPlatformResource(m.ModuleName, m.ModulePath, "module", m.ModuleName); pr != nil {
				node.PlatformResourceID = &pr.ID
				node.JumpURL = buildResourceJumpURL(workspaceID, pr, m.ModulePath, mInfo)
			} else if mInfo != nil {
				// manifest workspace：即使无 platform resource 也可跳编辑器
				node.JumpURL = buildResourceJumpURL(workspaceID, nil, m.ModulePath, mInfo)
			}
		}

		moduleNodes[m.ModulePath] = node
	}

	// 建立module父子关系
	for _, m := range modules {
		if m.ParentPath != "" {
			if parent, ok := moduleNodes[m.ParentPath]; ok {
				parent.Children = append(parent.Children, moduleNodes[m.ModulePath])
			}
		}
	}

	// 添加资源到对应的module
	for _, r := range resources {
		resourceNode := &models.ResourceTreeNode{
			Type:             "resource",
			Name:             r.ResourceName,
			TerraformAddress: r.TerraformAddress,
			TerraformType:    r.ResourceType,
			TerraformName:    r.ResourceName,
			CloudID:          r.CloudResourceID,
			CloudName:        r.CloudResourceName,
			CloudARN:         r.CloudResourceARN,
			Description:      r.Description,
			Mode:             r.ResourceMode,
			ResourceSummary:  r.ResourceSummary,
		}

		pr := findPlatformResource(r.RootModuleName, r.TerraformAddress, r.ResourceType, r.ResourceName)
		if pr != nil {
			resourceNode.PlatformResourceID = &pr.ID
		}
		if jump := buildResourceJumpURL(workspaceID, pr, r.TerraformAddress, mInfo); jump != "" {
			resourceNode.JumpURL = jump
		}

		if r.ModulePath != "" {
			if parent, ok := moduleNodes[r.ModulePath]; ok {
				parent.Children = append(parent.Children, resourceNode)
			}
		}
	}

	// 收集根节点
	var rootNodes []*models.ResourceTreeNode
	for _, m := range modules {
		if m.ParentPath == "" {
			rootNodes = append(rootNodes, moduleNodes[m.ModulePath])
		}
	}

	// 添加没有module的资源（直接在root下的资源）
	for _, r := range resources {
		if r.ModulePath == "" {
			resourceNode := &models.ResourceTreeNode{
				Type:             "resource",
				Name:             r.ResourceName,
				TerraformAddress: r.TerraformAddress,
				TerraformType:    r.ResourceType,
				TerraformName:    r.ResourceName,
				CloudID:          r.CloudResourceID,
				CloudName:        r.CloudResourceName,
				CloudARN:         r.CloudResourceARN,
				Description:      r.Description,
				Mode:             r.ResourceMode,
				ResourceSummary:  r.ResourceSummary,
			}
			pr := findPlatformResource(r.RootModuleName, r.TerraformAddress, r.ResourceType, r.ResourceName)
			if pr != nil {
				resourceNode.PlatformResourceID = &pr.ID
			}
			if jump := buildResourceJumpURL(workspaceID, pr, r.TerraformAddress, mInfo); jump != "" {
				resourceNode.JumpURL = jump
			}
			rootNodes = append(rootNodes, resourceNode)
		}
	}

	return rootNodes
}

// GetResourceDetail 获取资源详情
func (s *CMDBService) GetResourceDetail(workspaceID, terraformAddress string) (*models.ResourceIndex, error) {
	var resource models.ResourceIndex
	if err := s.db.Where("workspace_id = ? AND terraform_address = ?", workspaceID, terraformAddress).
		First(&resource).Error; err != nil {
		return nil, err
	}
	return &resource, nil
}

// GetCMDBStats 获取CMDB统计信息
func (s *CMDBService) GetCMDBStats() (*models.CMDBStats, error) {
	var stats models.CMDBStats

	// 统计workspace数量
	s.db.Model(&models.ResourceIndex{}).
		Select("COUNT(DISTINCT workspace_id)").
		Scan(&stats.TotalWorkspaces)

	// 统计资源数量
	s.db.Model(&models.ResourceIndex{}).
		Where("resource_mode = ?", "managed").
		Count(&stats.TotalResources)

	// 统计module数量
	s.db.Model(&models.ModuleHierarchy{}).Count(&stats.TotalModules)

	// 资源类型统计
	var typeStats []models.ResourceTypeStat
	s.db.Model(&models.ResourceIndex{}).
		Select("resource_type, COUNT(*) as count").
		Where("resource_mode = ?", "managed").
		Group("resource_type").
		Order("count DESC").
		Limit(10).
		Scan(&typeStats)
	stats.ResourceTypeStats = typeStats

	// 最后同步时间
	var lastSynced time.Time
	s.db.Model(&models.ResourceIndex{}).
		Select("MAX(last_synced_at)").
		Scan(&lastSynced)
	if !lastSynced.IsZero() {
		stats.LastSyncedAt = &lastSynced
	}

	return &stats, nil
}

// GetCMDBOverview 获取 CMDB 观测面板数据
func (s *CMDBService) GetCMDBOverview() (*models.CMDBOverview, error) {
	var overview models.CMDBOverview

	// === Sources ===
	s.db.Model(&models.ResourceIndex{}).
		Where("source_type = ?", "terraform").
		Select("COUNT(DISTINCT workspace_id)").
		Scan(&overview.Sources.WorkspaceCount)

	s.db.Model(&models.CMDBExternalSource{}).
		Where("is_enabled = ?", true).
		Count(&overview.Sources.ExternalSourceCount)
	s.db.Model(&models.CMDBExternalSource{}).
		Where("is_enabled = ? AND last_sync_status = ?", true, "success").
		Count(&overview.Sources.ExternalSourceHealthy)
	overview.Sources.ExternalSourceError = overview.Sources.ExternalSourceCount - overview.Sources.ExternalSourceHealthy

	// === Resources ===
	s.db.Model(&models.ResourceIndex{}).
		Where("resource_mode = ?", "managed").
		Count(&overview.Resources.Total)
	s.db.Model(&models.ResourceIndex{}).
		Where("resource_mode = ? AND source_type = ?", "managed", "terraform").
		Count(&overview.Resources.FromWorkspace)
	overview.Resources.FromExternal = overview.Resources.Total - overview.Resources.FromWorkspace

	typeStats := make([]models.ResourceTypeStat, 0)
	s.db.Model(&models.ResourceIndex{}).
		Select("resource_type, COUNT(*) as count").
		Where("resource_mode = ?", "managed").
		Group("resource_type").Order("count DESC").Limit(10).
		Scan(&typeStats)
	overview.Resources.TypeTop10 = typeStats

	// === Embedding ===
	overview.Embedding.Total = overview.Resources.Total
	s.db.Model(&models.ResourceIndex{}).
		Where("resource_mode = ? AND embedding IS NOT NULL", "managed").
		Count(&overview.Embedding.Completed)
	if overview.Embedding.Total > 0 {
		overview.Embedding.CoveragePct = float64(overview.Embedding.Completed) / float64(overview.Embedding.Total) * 100
	}

	// === Summary ===
	overview.Summary.Total = overview.Resources.Total
	s.db.Model(&models.ResourceIndex{}).
		Where("resource_mode = ? AND resource_summary IS NOT NULL AND resource_summary != ''", "managed").
		Count(&overview.Summary.Completed)
	if overview.Summary.Total > 0 {
		overview.Summary.CoveragePct = float64(overview.Summary.Completed) / float64(overview.Summary.Total) * 100
	}

	// === Queue ===
	// Workspace embedding 任务队列 (embedding_tasks)
	s.db.Model(&models.EmbeddingTask{}).Where("status = ?", "pending").Count(&overview.Queue.EmbeddingPending)
	s.db.Model(&models.EmbeddingTask{}).Where("status = ?", "processing").Count(&overview.Queue.EmbeddingProcessing)
	s.db.Model(&models.EmbeddingTask{}).Where("status = ?", "failed").Count(&overview.Queue.EmbeddingFailed)
	// 外部源 summary 任务队列 (cmdb_post_sync_jobs, job_type=summary)
	s.db.Model(&models.PostSyncJob{}).Where("job_type = ? AND status = ?", "summary", "pending").Count(&overview.Queue.SummaryPending)
	s.db.Model(&models.PostSyncJob{}).Where("job_type = ? AND status = ?", "summary", "processing").Count(&overview.Queue.SummaryProcessing)
	s.db.Model(&models.PostSyncJob{}).Where("job_type = ? AND status = ?", "summary", "failed").Count(&overview.Queue.SummaryFailed)
	// 外部源 embedding 任务队列 (cmdb_post_sync_jobs, job_type=embedding)
	s.db.Model(&models.PostSyncJob{}).Where("job_type = ? AND status = ?", "embedding", "pending").Count(&overview.Queue.ExtEmbeddingPending)
	s.db.Model(&models.PostSyncJob{}).Where("job_type = ? AND status = ?", "embedding", "processing").Count(&overview.Queue.ExtEmbeddingProcessing)
	s.db.Model(&models.PostSyncJob{}).Where("job_type = ? AND status = ?", "embedding", "failed").Count(&overview.Queue.ExtEmbeddingFailed)
	// 外部源摘要评估任务队列 (cmdb_post_sync_jobs, job_type=summary_assessment)
	s.db.Model(&models.PostSyncJob{}).Where("job_type = ? AND status = ?", models.PostSyncJobTypeSummaryAssessment, "pending").Count(&overview.Queue.AssessmentPending)
	s.db.Model(&models.PostSyncJob{}).Where("job_type = ? AND status = ?", models.PostSyncJobTypeSummaryAssessment, "processing").Count(&overview.Queue.AssessmentProcessing)
	s.db.Model(&models.PostSyncJob{}).Where("job_type = ? AND status = ?", models.PostSyncJobTypeSummaryAssessment, "failed").Count(&overview.Queue.AssessmentFailed)
	// 资源级别：L2/L3 评估待补偿
	s.db.Model(&models.ResourceIndex{}).Where("summary_assessment_status = ?", string(models.AssessmentStatusPartial)).Count(&overview.Queue.AssessmentPartial)

	return &overview, nil
}

// GetSyncHistory 获取同步历史（分页）
func (s *CMDBService) GetSyncHistory(page, size int) (*models.CMDBSyncHistoryResponse, error) {
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 10
	}

	var total int64
	s.db.Table("cmdb_sync_logs").Count(&total)

	syncs := make([]models.CMDBRecentSync, 0)
	s.db.Table("cmdb_sync_logs").
		Order("started_at DESC").
		Offset((page - 1) * size).
		Limit(size).
		Scan(&syncs)

	return &models.CMDBSyncHistoryResponse{
		Syncs: syncs,
		Total: total,
		Page:  page,
		Size:  size,
	}, nil
}

// SyncAllWorkspaces 同步所有workspace的资源索引
func (s *CMDBService) SyncAllWorkspaces() error {
	var workspaces []models.Workspace
	if err := s.db.Find(&workspaces).Error; err != nil {
		return err
	}

	for _, ws := range workspaces {
		if err := s.SyncWorkspaceResources(ws.WorkspaceID); err != nil {
			// 记录错误但继续处理其他workspace
			log.Printf("[CMDB] Failed to sync workspace %s: %v", ws.WorkspaceID, err)
		}
	}

	return nil
}

// GetWorkspaceResourceCounts 获取所有workspace的资源数量统计
func (s *CMDBService) GetWorkspaceResourceCounts() ([]models.WorkspaceResourceCount, error) {
	var counts []models.WorkspaceResourceCount

	err := s.db.Table("resource_index ri").
		Select(`
			ri.workspace_id,
			w.name as workspace_name,
			COUNT(*) as resource_count,
			MAX(ri.last_synced_at) as last_synced_at
		`).
		Joins("LEFT JOIN workspaces w ON ri.workspace_id = w.workspace_id").
		Where("ri.resource_mode = ?", "managed").
		Group("ri.workspace_id, w.name").
		Order("w.name").
		Scan(&counts).Error

	if err != nil {
		return nil, err
	}

	return counts, nil
}

// SearchSuggestion 搜索建议项
type SearchSuggestion struct {
	Value        string `json:"value"`                 // 建议值（用于搜索）
	Label        string `json:"label"`                 // 显示标签
	Type         string `json:"type"`                  // 类型：id, name, description, arn
	ResourceType string `json:"resource_type"`         // 资源类型
	SourceType   string `json:"source_type,omitempty"` // 数据源类型：terraform 或 external
	IsExternal   bool   `json:"is_external,omitempty"` // 是否为外部数据源
}

// CMDBFieldDefinition CMDB字段定义
type CMDBFieldDefinition struct {
	Key         string   `json:"key"`         // 字段Key（如 cloud_id）
	Label       string   `json:"label"`       // 显示名称（如 "资源 ID"）
	Description string   `json:"description"` // 字段说明
	Examples    []string `json:"examples"`    // 示例值列表
}

// CMDBResourceOption CMDB资源选项
type CMDBResourceOption struct {
	Value         string            `json:"value"`                    // 选项值（根据 value_field 提取）
	Label         string            `json:"label"`                    // 显示标签（资源名称）
	Description   string            `json:"description,omitempty"`    // 资源描述
	WorkspaceID   string            `json:"workspace_id,omitempty"`   // 所属 workspace ID
	WorkspaceName string            `json:"workspace_name,omitempty"` // 所属 workspace 名称
	SourceType    string            `json:"source_type,omitempty"`    // 数据源类型
	Extra         map[string]string `json:"extra,omitempty"`          // 额外信息
}

// CMDBOptionsResponse CMDB选项响应
type CMDBOptionsResponse struct {
	Options []CMDBResourceOption `json:"options"`
	Total   int64                `json:"total"`
	HasMore bool                 `json:"has_more"`
}

// 预定义的CMDB字段列表
var cmdbFieldDefinitions = []CMDBFieldDefinition{
	{
		Key:         "cloud_id",
		Label:       "资源 ID",
		Description: "云资源唯一标识符",
		Examples:    []string{"sg-0123456789abcdef0", "subnet-0123456789abcdef0", "vpc-0123456789abcdef0"},
	},
	{
		Key:         "cloud_arn",
		Label:       "ARN",
		Description: "AWS ARN / Azure Resource ID",
		Examples:    []string{"arn:aws:iam::123456789012:role/my-role", "arn:aws:s3:::my-bucket"},
	},
	{
		Key:         "cloud_name",
		Label:       "资源名称",
		Description: "云资源的名称",
		Examples:    []string{"my-instance", "production-db", "web-server"},
	},
	{
		Key:         "cloud_region",
		Label:       "区域",
		Description: "云资源所在区域",
		Examples:    []string{"us-east-1", "ap-southeast-1", "eu-west-1"},
	},
	{
		Key:         "cloud_account",
		Label:       "账户 ID",
		Description: "云账户标识符",
		Examples:    []string{"123456789012", "987654321098"},
	},
	{
		Key:         "terraform_address",
		Label:       "Terraform 地址",
		Description: "完整的 Terraform 资源地址",
		Examples:    []string{"module.vpc.aws_vpc.this[0]", "aws_instance.web"},
	},
	{
		Key:         "description",
		Label:       "描述",
		Description: "资源描述信息",
		Examples:    []string{"Production database server", "Web application load balancer"},
	},
}

// 资源类型推荐的valueField映射
var resourceTypeRecommendedFields = map[string]string{
	"aws_security_group":       "cloud_id",
	"aws_iam_role":             "cloud_arn",
	"aws_iam_policy":           "cloud_arn",
	"aws_iam_instance_profile": "cloud_arn",
	"aws_subnet":               "cloud_id",
	"aws_vpc":                  "cloud_id",
	"aws_s3_bucket":            "cloud_id",
	"aws_kms_key":              "cloud_arn",
	"aws_lb":                   "cloud_arn",
	"aws_lb_target_group":      "cloud_arn",
	"aws_ami":                  "cloud_id",
	"aws_key_pair":             "cloud_name",
	"aws_acm_certificate":      "cloud_arn",
	"aws_eks_cluster":          "cloud_name",
	"aws_rds_cluster":          "cloud_id",
	"aws_db_instance":          "cloud_id",
}

// GetCMDBFieldDefinitions 获取CMDB字段定义列表
func (s *CMDBService) GetCMDBFieldDefinitions() []CMDBFieldDefinition {
	return cmdbFieldDefinitions
}

// GetRecommendedValueField 获取资源类型推荐的valueField
func (s *CMDBService) GetRecommendedValueField(resourceType string) string {
	if field, ok := resourceTypeRecommendedFields[resourceType]; ok {
		return field
	}
	return "cloud_id" // 默认返回 cloud_id
}

// GetCMDBResourceOptions 获取CMDB资源选项列表
func (s *CMDBService) GetCMDBResourceOptions(resourceType, valueField, query, workspaceID string, limit int) (*CMDBOptionsResponse, error) {
	if limit <= 0 {
		limit = 50
	}

	// 验证valueField是否有效
	validField := false
	for _, f := range cmdbFieldDefinitions {
		if f.Key == valueField {
			validField = true
			break
		}
	}
	if !validField {
		return nil, fmt.Errorf("invalid value_field: %s", valueField)
	}

	// 构建查询
	db := s.db.Table("resource_index ri").
		Select(`
			ri.cloud_resource_id,
			ri.cloud_resource_name,
			ri.cloud_resource_arn,
			ri.cloud_region,
			ri.cloud_account_id,
			ri.terraform_address,
			ri.description,
			ri.workspace_id,
			w.name as workspace_name,
			ri.source_type
		`).
		Joins("LEFT JOIN workspaces w ON ri.workspace_id = w.workspace_id").
		Where("ri.resource_mode = ?", "managed").
		Where("ri.resource_type = ?", resourceType)

	if workspaceID != "" {
		db = db.Where("ri.workspace_id = ?", workspaceID)
	}

	if query != "" {
		searchPattern := "%" + query + "%"
		db = db.Where(`
			ri.cloud_resource_id ILIKE ? OR
			ri.cloud_resource_name ILIKE ? OR
			ri.cloud_resource_arn ILIKE ? OR
			ri.description ILIKE ? OR
			ri.tags::text ILIKE ?
		`, searchPattern, searchPattern, searchPattern, searchPattern, searchPattern)
	}

	// 获取总数
	var total int64
	countDB := s.db.Table("resource_index ri").
		Where("ri.resource_mode = ?", "managed").
		Where("ri.resource_type = ?", resourceType)
	if workspaceID != "" {
		countDB = countDB.Where("ri.workspace_id = ?", workspaceID)
	}
	if query != "" {
		searchPattern := "%" + query + "%"
		countDB = countDB.Where(`
			ri.cloud_resource_id ILIKE ? OR
			ri.cloud_resource_name ILIKE ? OR
			ri.cloud_resource_arn ILIKE ? OR
			ri.description ILIKE ? OR
			ri.tags::text ILIKE ?
		`, searchPattern, searchPattern, searchPattern, searchPattern, searchPattern)
	}
	countDB.Count(&total)

	// 查询结果
	var results []struct {
		CloudResourceID   string `gorm:"column:cloud_resource_id"`
		CloudResourceName string `gorm:"column:cloud_resource_name"`
		CloudResourceARN  string `gorm:"column:cloud_resource_arn"`
		CloudRegion       string `gorm:"column:cloud_region"`
		CloudAccountID    string `gorm:"column:cloud_account_id"`
		TerraformAddress  string `gorm:"column:terraform_address"`
		Description       string `gorm:"column:description"`
		WorkspaceID       string `gorm:"column:workspace_id"`
		WorkspaceName     string `gorm:"column:workspace_name"`
		SourceType        string `gorm:"column:source_type"`
	}

	if err := db.Order("ri.cloud_resource_name").Limit(limit).Scan(&results).Error; err != nil {
		return nil, err
	}

	// 构建选项列表
	options := make([]CMDBResourceOption, 0, len(results))
	for _, r := range results {
		// 根据valueField提取值
		value := s.extractValueByField(r.CloudResourceID, r.CloudResourceName, r.CloudResourceARN,
			r.CloudRegion, r.CloudAccountID, r.TerraformAddress, r.Description, valueField)

		if value == "" {
			continue // 跳过空值
		}

		// 构建显示标签
		label := r.CloudResourceName
		if label == "" {
			label = r.CloudResourceID
		}

		option := CMDBResourceOption{
			Value:         value,
			Label:         label,
			Description:   r.Description,
			WorkspaceID:   r.WorkspaceID,
			WorkspaceName: r.WorkspaceName,
			SourceType:    r.SourceType,
			Extra: map[string]string{
				"cloud_id":   r.CloudResourceID,
				"cloud_arn":  r.CloudResourceARN,
				"cloud_name": r.CloudResourceName,
			},
		}
		options = append(options, option)
	}

	return &CMDBOptionsResponse{
		Options: options,
		Total:   total,
		HasMore: int64(len(options)) < total,
	}, nil
}

// extractValueByField 根据valueField提取对应的值
func (s *CMDBService) extractValueByField(cloudID, cloudName, cloudARN, cloudRegion, cloudAccount, terraformAddress, description, valueField string) string {
	switch valueField {
	case "cloud_id":
		return cloudID
	case "cloud_arn":
		return cloudARN
	case "cloud_name":
		return cloudName
	case "cloud_region":
		return cloudRegion
	case "cloud_account":
		return cloudAccount
	case "terraform_address":
		return terraformAddress
	case "description":
		return description
	default:
		return cloudID
	}
}

// GetSearchSuggestions 获取搜索建议
func (s *CMDBService) GetSearchSuggestions(prefix string, limit int) ([]SearchSuggestion, error) {
	if limit <= 0 {
		limit = 10
	}
	if prefix == "" {
		return []SearchSuggestion{}, nil
	}

	var suggestions []SearchSuggestion
	searchPattern := prefix + "%"
	containsPattern := "%" + prefix + "%"

	// 1. 搜索 cloud_resource_id（精确前缀匹配优先）
	var idResults []struct {
		CloudResourceID string `gorm:"column:cloud_resource_id"`
		ResourceType    string `gorm:"column:resource_type"`
		SourceType      string `gorm:"column:source_type"`
	}
	s.db.Table("resource_index").
		Select("DISTINCT cloud_resource_id, resource_type, source_type").
		Where("resource_mode = ? AND cloud_resource_id ILIKE ?", "managed", searchPattern).
		Limit(limit).
		Scan(&idResults)

	for _, r := range idResults {
		if r.CloudResourceID != "" {
			suggestions = append(suggestions, SearchSuggestion{
				Value:        r.CloudResourceID,
				Label:        r.CloudResourceID,
				Type:         "id",
				ResourceType: r.ResourceType,
				SourceType:   r.SourceType,
				IsExternal:   r.SourceType == "external",
			})
		}
	}

	// 2. 搜索 cloud_resource_arn（如果还有空间）
	remaining := limit - len(suggestions)
	if remaining > 0 {
		var arnResults []struct {
			CloudResourceARN string `gorm:"column:cloud_resource_arn"`
			ResourceType     string `gorm:"column:resource_type"`
			SourceType       string `gorm:"column:source_type"`
		}
		s.db.Table("resource_index").
			Select("DISTINCT cloud_resource_arn, resource_type, source_type").
			Where("resource_mode = ? AND cloud_resource_arn ILIKE ? AND cloud_resource_arn != ''", "managed", containsPattern).
			Limit(remaining).
			Scan(&arnResults)

		for _, r := range arnResults {
			if r.CloudResourceARN != "" {
				label := r.CloudResourceARN
				if len(label) > 60 {
					label = label[:60] + "..."
				}
				suggestions = append(suggestions, SearchSuggestion{
					Value:        r.CloudResourceARN,
					Label:        label,
					Type:         "arn",
					ResourceType: r.ResourceType,
					SourceType:   r.SourceType,
					IsExternal:   r.SourceType == "external",
				})
			}
		}
	}

	// 3. 搜索 cloud_resource_name（如果还有空间）
	remaining = limit - len(suggestions)
	if remaining > 0 {
		var nameResults []struct {
			CloudResourceName string `gorm:"column:cloud_resource_name"`
			ResourceType      string `gorm:"column:resource_type"`
			SourceType        string `gorm:"column:source_type"`
		}
		s.db.Table("resource_index").
			Select("DISTINCT cloud_resource_name, resource_type, source_type").
			Where("resource_mode = ? AND cloud_resource_name ILIKE ? AND cloud_resource_name != ''", "managed", containsPattern).
			Limit(remaining).
			Scan(&nameResults)

		for _, r := range nameResults {
			if r.CloudResourceName != "" {
				suggestions = append(suggestions, SearchSuggestion{
					Value:        r.CloudResourceName,
					Label:        r.CloudResourceName,
					Type:         "name",
					ResourceType: r.ResourceType,
					SourceType:   r.SourceType,
					IsExternal:   r.SourceType == "external",
				})
			}
		}
	}

	// 4. 搜索 description（如果还有空间）
	remaining = limit - len(suggestions)
	if remaining > 0 {
		var descResults []struct {
			Description  string `gorm:"column:description"`
			ResourceType string `gorm:"column:resource_type"`
			SourceType   string `gorm:"column:source_type"`
		}
		s.db.Table("resource_index").
			Select("DISTINCT description, resource_type, source_type").
			Where("resource_mode = ? AND description ILIKE ? AND description != ''", "managed", containsPattern).
			Limit(remaining).
			Scan(&descResults)

		for _, r := range descResults {
			if r.Description != "" {
				label := r.Description
				if len(label) > 50 {
					label = label[:50] + "..."
				}
				suggestions = append(suggestions, SearchSuggestion{
					Value:        r.Description,
					Label:        label,
					Type:         "description",
					ResourceType: r.ResourceType,
					SourceType:   r.SourceType,
					IsExternal:   r.SourceType == "external",
				})
			}
		}
	}

	return suggestions, nil
}

// GetSearchAnalytics 获取搜索分析聚合数据
func (s *CMDBService) GetSearchAnalytics(period, source string) (*models.CMDBSearchAnalytics, error) {
	// 解析时间范围（白名单，防止注入）
	var interval string
	switch period {
	case "24h":
		interval = "1 day"
	case "30d":
		interval = "30 days"
	default:
		period = "7d"
		interval = "7 days"
	}

	// 构建参数化的 source 条件
	// source 只允许 manual/auto/all，用白名单而非拼接用户输入
	var sourceFilter string
	var args []interface{}
	since := fmt.Sprintf("NOW() - INTERVAL '%s'", interval) // interval 来自白名单 switch，安全

	switch source {
	case "all":
		sourceFilter = "created_at >= " + since
	case "auto", "agent", "manual":
		sourceFilter = "created_at >= " + since + " AND source = $1"
		args = append(args, source)
	default:
		sourceFilter = "created_at >= " + since + " AND source = $1"
		args = append(args, "manual")
	}

	result := &models.CMDBSearchAnalytics{Period: period}

	// 1. Usage 统计
	var usage struct {
		TotalSearches   int64   `gorm:"column:total_searches"`
		ZeroResultCount int64   `gorm:"column:zero_result_count"`
		AvgResultCount  float64 `gorm:"column:avg_result_count"`
		UniqueQueries   int64   `gorm:"column:unique_queries"`
	}
	if err := s.db.Raw(`
		SELECT
			COUNT(*) AS total_searches,
			COUNT(*) FILTER (WHERE total_count = 0) AS zero_result_count,
			COALESCE(ROUND(AVG(total_count)::numeric, 1), 0) AS avg_result_count,
			COUNT(DISTINCT query) AS unique_queries
		FROM cmdb_search_logs
		WHERE `+sourceFilter,
		args...).Scan(&usage).Error; err != nil {
		return nil, fmt.Errorf("search analytics usage query failed: %w", err)
	}

	result.Usage = models.CMDBSearchUsage{
		TotalSearches:   usage.TotalSearches,
		ZeroResultCount: usage.ZeroResultCount,
		AvgResultCount:  usage.AvgResultCount,
		UniqueQueries:   usage.UniqueQueries,
	}
	if usage.TotalSearches > 0 {
		result.Usage.ZeroResultRate = float64(usage.ZeroResultCount) / float64(usage.TotalSearches) * 100
	}

	// 2. Quality 统计
	var quality struct {
		MethodHybrid     int64   `gorm:"column:method_hybrid"`
		MethodVector     int64   `gorm:"column:method_vector"`
		MethodKeyword    int64   `gorm:"column:method_keyword"`
		AvgTopSimilarity float64 `gorm:"column:avg_top_similarity"`
		AvgAvgSimilarity float64 `gorm:"column:avg_avg_similarity"`
		AvgDurationMs    float64 `gorm:"column:avg_duration_ms"`
		FallbackCount    int64   `gorm:"column:fallback_count"`
	}
	if err := s.db.Raw(`
		SELECT
			COUNT(*) FILTER (WHERE search_method = 'hybrid') AS method_hybrid,
			COUNT(*) FILTER (WHERE search_method = 'vector') AS method_vector,
			COUNT(*) FILTER (WHERE search_method = 'keyword') AS method_keyword,
			COALESCE(ROUND(AVG(top_similarity) FILTER (WHERE vector_count > 0)::numeric, 3), 0) AS avg_top_similarity,
			COALESCE(ROUND(AVG(avg_similarity) FILTER (WHERE vector_count > 0)::numeric, 3), 0) AS avg_avg_similarity,
			COALESCE(ROUND(AVG(duration_ms)::numeric, 0), 0) AS avg_duration_ms,
			COUNT(*) FILTER (WHERE search_method = 'keyword' AND fallback_reason != '') AS fallback_count
		FROM cmdb_search_logs
		WHERE `+sourceFilter,
		args...).Scan(&quality).Error; err != nil {
		return nil, fmt.Errorf("search analytics quality query failed: %w", err)
	}

	result.Quality = models.CMDBSearchQuality{
		MethodDistribution: map[string]int64{
			"hybrid":  quality.MethodHybrid,
			"vector":  quality.MethodVector,
			"keyword": quality.MethodKeyword,
		},
		AvgTopSimilarity: quality.AvgTopSimilarity,
		AvgSimilarity:    quality.AvgAvgSimilarity,
		AvgDurationMs:    quality.AvgDurationMs,
	}
	if usage.TotalSearches > 0 {
		result.Quality.FallbackRate = float64(quality.FallbackCount) / float64(usage.TotalSearches) * 100
	}

	// 3. Top queries（Top 30，供词云使用）
	var topQueries []models.CMDBSearchQueryStat
	if err := s.db.Raw(`
		SELECT query, COUNT(*) AS count, ROUND(AVG(total_count)::numeric, 1) AS avg_results
		FROM cmdb_search_logs
		WHERE `+sourceFilter+`
		GROUP BY query ORDER BY count DESC LIMIT 30`,
		args...).Scan(&topQueries).Error; err != nil {
		return nil, fmt.Errorf("search analytics top queries failed: %w", err)
	}
	if topQueries == nil {
		topQueries = []models.CMDBSearchQueryStat{}
	}
	result.TopQueries = topQueries

	// 4. Zero result queries（Top 10）
	var zeroResultQueries []models.CMDBSearchZeroResultStat
	if err := s.db.Raw(`
		SELECT query, COUNT(*) AS count, MAX(created_at) AS last_at
		FROM cmdb_search_logs
		WHERE `+sourceFilter+` AND total_count = 0
		GROUP BY query ORDER BY count DESC LIMIT 10`,
		args...).Scan(&zeroResultQueries).Error; err != nil {
		return nil, fmt.Errorf("search analytics zero result queries failed: %w", err)
	}
	if zeroResultQueries == nil {
		zeroResultQueries = []models.CMDBSearchZeroResultStat{}
	}
	result.ZeroResultQueries = zeroResultQueries

	return result, nil
}
