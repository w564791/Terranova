package services

import (
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
func (s *CMDBService) SyncWorkspaceResources(workspaceID string) error {
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

	// 2. Content已经是JSONB类型（map[string]interface{}），直接使用
	stateContent := map[string]interface{}(stateVersion.Content)

	// 3. 解析并同步资源
	if err := s.parseAndSyncState(workspaceID, stateContent, stateVersion.ID); err != nil {
		return err
	}

	// 4. 触发 embedding 生成（异步，不阻塞主流程）
	go func() {
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

// parseAndSyncState 解析State并同步到资源索引
// 使用增量更新模式，保留已有的 embedding 数据
func (s *CMDBService) parseAndSyncState(workspaceID string, stateContent map[string]interface{}, stateVersionID uint) error {
	// 1. 解析resources数组
	resources, ok := stateContent["resources"].([]interface{})
	if !ok {
		return nil // 空state
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
	return s.db.Transaction(func(tx *gorm.DB) error {
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
			} else {
				// 创建新记录
				if err := tx.Create(&record).Error; err != nil {
					return err
				}
			}
		}

		// 删除不再存在的资源
		for address, existingRecord := range existingResources {
			if !processedAddresses[address] {
				if err := tx.Delete(existingRecord).Error; err != nil {
					return err
				}
			}
		}

		// 更新module层级表
		return s.syncModuleHierarchy(tx, workspaceID, moduleSet)
	})
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

// SearchResources 搜索资源
func (s *CMDBService) SearchResources(query string, workspaceID string, resourceType string, limit int) ([]models.ResourceSearchResult, error) {
	if limit <= 0 {
		limit = 20
	}

	var results []models.ResourceSearchResult

	// 构建查询 - 使用模糊匹配关联workspace_resources和cmdb_external_sources
	db := s.db.Table("resource_index ri").
		Select(`
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
			wr.id as platform_resource_id,
			wr.resource_name as platform_resource_name,
			CASE 
				WHEN ri.source_type = 'external' THEN NULL
				WHEN wr.id IS NOT NULL THEN CONCAT('/workspaces/', ri.workspace_id, '/resources/', wr.id)
				ELSE NULL
			END as jump_url,
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
		`, query, query, query, query+"%", query+"%", query+"%", "%"+query+"%", "%"+query+"%", "%"+query+"%", "%"+query+"%", "%"+query+"%").
		Joins("LEFT JOIN workspace_resources wr ON ri.workspace_id = wr.workspace_id AND ri.source_type = 'terraform' AND (ri.root_module_name LIKE '%' || wr.resource_name || '%' OR wr.resource_name LIKE '%' || ri.root_module_name || '%') AND wr.is_active = true").
		Joins("LEFT JOIN workspaces w ON ri.workspace_id = w.workspace_id").
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

	// 排序：内部数据（terraform）优先，然后按匹配度排序
	if err := db.Order("ri.source_type ASC, match_rank DESC, ri.cloud_resource_name").
		Limit(limit).
		Scan(&results).Error; err != nil {
		return nil, err
	}

	return results, nil
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

	// 获取平台资源映射
	platformResources := make(map[string]*models.WorkspaceResource)
	var wsResources []models.WorkspaceResource
	if err := s.db.Where("workspace_id = ? AND is_active = true", workspaceID).Find(&wsResources).Error; err == nil {
		for i := range wsResources {
			platformResources[wsResources[i].ResourceName] = &wsResources[i]
		}
	}

	// 构建树结构
	tree := s.buildResourceTree(modules, resources, platformResources, workspaceID)

	return &models.WorkspaceResourceTree{
		WorkspaceID:    workspaceID,
		WorkspaceName:  workspace.Name,
		TotalResources: len(resources),
		Tree:           tree,
	}, nil
}

// buildResourceTree 构建资源树
func (s *CMDBService) buildResourceTree(modules []models.ModuleHierarchy, resources []models.ResourceIndex, platformResources map[string]*models.WorkspaceResource, workspaceID string) []*models.ResourceTreeNode {
	// 创建module节点映射
	moduleNodes := make(map[string]*models.ResourceTreeNode)

	// 为根module查找对应的平台资源（使用模糊匹配）
	findPlatformResource := func(moduleName string) *models.WorkspaceResource {
		// 精确匹配
		if pr, ok := platformResources[moduleName]; ok {
			return pr
		}
		// 模糊匹配：检查resource_name是否包含在moduleName中
		for resName, pr := range platformResources {
			if strings.Contains(moduleName, resName) || strings.Contains(resName, moduleName) {
				return pr
			}
		}
		return nil
	}

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
			if pr := findPlatformResource(m.ModuleName); pr != nil {
				node.PlatformResourceID = &pr.ID
				node.JumpURL = fmt.Sprintf("/workspaces/%s/resources/%d", workspaceID, pr.ID)
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
		}

		// 添加平台资源跳转链接（使用模糊匹配）
		if r.RootModuleName != "" {
			if pr := findPlatformResource(r.RootModuleName); pr != nil {
				resourceNode.PlatformResourceID = &pr.ID
				resourceNode.JumpURL = fmt.Sprintf("/workspaces/%s/resources/%d", workspaceID, pr.ID)
			}
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
