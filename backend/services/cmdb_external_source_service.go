package services

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"iac-platform/internal/crypto"
	"iac-platform/internal/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// CMDBExternalSourceService 外部CMDB数据源服务
type CMDBExternalSourceService struct {
	db         *gorm.DB
	httpClient *http.Client
}

// NewCMDBExternalSourceService 创建外部数据源服务
func NewCMDBExternalSourceService(db *gorm.DB) *CMDBExternalSourceService {
	return &CMDBExternalSourceService{
		db: db,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// generateSecretID 生成密钥ID
func generateSecretID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return "secret-" + hex.EncodeToString(b)
}

// GenerateSourceID 生成数据源ID
func GenerateSourceID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return "cmdb-src-" + hex.EncodeToString(b)
}

// CreateExternalSource 创建外部数据源
func (s *CMDBExternalSourceService) CreateExternalSource(ctx context.Context, req *models.CreateExternalSourceRequest, userID string) (*models.CMDBExternalSource, error) {
	// 1. 生成source_id
	sourceID := GenerateSourceID()

	// 2. 处理auth_headers - 将value加密存储到secrets表
	var authHeaders []models.AuthHeader
	for _, h := range req.AuthHeaders {
		header := models.AuthHeader{
			Key: h.Key,
		}

		// 如果提供了value，加密存储
		if h.Value != nil && *h.Value != "" {
			secretID, err := s.storeHeaderSecret(sourceID, h.Key, *h.Value, userID)
			if err != nil {
				return nil, fmt.Errorf("failed to store header secret: %w", err)
			}
			header.SecretID = secretID
		}

		authHeaders = append(authHeaders, header)
	}

	// 3. 序列化auth_headers
	authHeadersJSON, err := json.Marshal(authHeaders)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal auth_headers: %w", err)
	}

	// 4. 序列化field_mapping
	fieldMappingJSON, err := json.Marshal(req.FieldMapping)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal field_mapping: %w", err)
	}

	// 5. 创建数据源记录
	source := &models.CMDBExternalSource{
		SourceID:            sourceID,
		Name:                req.Name,
		Description:         req.Description,
		APIEndpoint:         req.APIEndpoint,
		HTTPMethod:          req.HTTPMethod,
		RequestBody:         req.RequestBody,
		AuthHeaders:         authHeadersJSON,
		ResponsePath:        req.ResponsePath,
		FieldMapping:        fieldMappingJSON,
		PrimaryKeyField:     req.PrimaryKeyField,
		CloudProvider:       req.CloudProvider,
		AccountID:           req.AccountID,
		AccountName:         req.AccountName,
		Region:              req.Region,
		SyncIntervalMinutes: req.SyncIntervalMinutes,
		IsEnabled:           true,
		ResourceTypeFilter:  req.ResourceTypeFilter,
		CreatedBy:           userID,
		UpdatedBy:           userID,
	}

	if source.HTTPMethod == "" {
		source.HTTPMethod = "GET"
	}

	if err := s.db.Create(source).Error; err != nil {
		return nil, fmt.Errorf("failed to create external source: %w", err)
	}

	return source, nil
}

// storeHeaderSecret 存储Header密钥到secrets表
func (s *CMDBExternalSourceService) storeHeaderSecret(sourceID, key, value, userID string) (string, error) {
	secretID := generateSecretID()

	// 使用crypto包加密存储
	metadata := map[string]interface{}{
		"key":         key,
		"description": fmt.Sprintf("CMDB External Source Header: %s", key),
	}
	metadataJSON, _ := json.Marshal(metadata)

	secret := &models.Secret{
		SecretID:     secretID,
		SecretType:   "api_header",
		ResourceType: "cmdb_external_source",
		ResourceID:   &sourceID,
		CreatedBy:    &userID,
		UpdatedBy:    &userID,
		IsActive:     true,
		Metadata:     metadataJSON,
	}

	// 加密value
	encryptedValue, err := crypto.EncryptValue(value)
	if err != nil {
		return "", fmt.Errorf("failed to encrypt header value: %w", err)
	}
	secret.ValueHash = encryptedValue

	if err := s.db.Create(secret).Error; err != nil {
		return "", fmt.Errorf("failed to create secret: %w", err)
	}

	return secretID, nil
}

// UpdateExternalSource 更新外部数据源
func (s *CMDBExternalSourceService) UpdateExternalSource(ctx context.Context, sourceID string, req *models.UpdateExternalSourceRequest, userID string) (*models.CMDBExternalSource, error) {
	// 1. 获取现有数据源
	var source models.CMDBExternalSource
	if err := s.db.Where("source_id = ?", sourceID).First(&source).Error; err != nil {
		return nil, fmt.Errorf("external source not found: %w", err)
	}

	// 2. 更新基本字段
	updates := make(map[string]interface{})
	if req.Name != nil {
		updates["name"] = *req.Name
	}
	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if req.APIEndpoint != nil {
		updates["api_endpoint"] = *req.APIEndpoint
	}
	if req.HTTPMethod != nil {
		updates["http_method"] = *req.HTTPMethod
	}
	if req.RequestBody != nil {
		updates["request_body"] = *req.RequestBody
	}
	if req.ResponsePath != nil {
		updates["response_path"] = *req.ResponsePath
	}
	if req.PrimaryKeyField != nil {
		updates["primary_key_field"] = *req.PrimaryKeyField
	}
	if req.CloudProvider != nil {
		updates["cloud_provider"] = *req.CloudProvider
	}
	if req.AccountID != nil {
		updates["account_id"] = *req.AccountID
	}
	if req.AccountName != nil {
		updates["account_name"] = *req.AccountName
	}
	if req.Region != nil {
		updates["region"] = *req.Region
	}
	if req.SyncIntervalMinutes != nil {
		updates["sync_interval_minutes"] = *req.SyncIntervalMinutes
	}
	if req.IsEnabled != nil {
		updates["is_enabled"] = *req.IsEnabled
	}
	if req.ResourceTypeFilter != nil {
		updates["resource_type_filter"] = *req.ResourceTypeFilter
	}
	if req.FieldMapping != nil {
		fieldMappingJSON, _ := json.Marshal(req.FieldMapping)
		updates["field_mapping"] = fieldMappingJSON
	}

	// 3. 处理auth_headers更新
	if req.AuthHeaders != nil {
		// 获取现有的headers
		existingHeaders, _ := source.GetAuthHeaders()
		existingHeaderMap := make(map[string]models.AuthHeader)
		for _, h := range existingHeaders {
			existingHeaderMap[h.Key] = h
		}

		var newHeaders []models.AuthHeader
		for _, h := range req.AuthHeaders {
			header := models.AuthHeader{
				Key: h.Key,
			}

			if h.Value != nil {
				// 提供了新值，更新或创建secret
				if existing, ok := existingHeaderMap[h.Key]; ok && existing.SecretID != "" {
					// 更新现有secret
					if err := s.updateHeaderSecret(existing.SecretID, *h.Value, userID); err != nil {
						return nil, fmt.Errorf("failed to update header secret: %w", err)
					}
					header.SecretID = existing.SecretID
				} else {
					// 创建新secret
					secretID, err := s.storeHeaderSecret(sourceID, h.Key, *h.Value, userID)
					if err != nil {
						return nil, fmt.Errorf("failed to store header secret: %w", err)
					}
					header.SecretID = secretID
				}
			} else {
				// 没有提供新值，保留现有secret
				if existing, ok := existingHeaderMap[h.Key]; ok {
					header.SecretID = existing.SecretID
				}
			}

			newHeaders = append(newHeaders, header)
		}

		// 删除不再使用的secrets
		for key, existing := range existingHeaderMap {
			found := false
			for _, h := range req.AuthHeaders {
				if h.Key == key {
					found = true
					break
				}
			}
			if !found && existing.SecretID != "" {
				s.db.Where("secret_id = ?", existing.SecretID).Delete(&models.Secret{})
			}
		}

		authHeadersJSON, _ := json.Marshal(newHeaders)
		updates["auth_headers"] = authHeadersJSON
	}

	updates["updated_by"] = userID
	updates["updated_at"] = time.Now()

	if err := s.db.Model(&source).Updates(updates).Error; err != nil {
		return nil, fmt.Errorf("failed to update external source: %w", err)
	}

	// 重新获取更新后的数据
	if err := s.db.Where("source_id = ?", sourceID).First(&source).Error; err != nil {
		return nil, err
	}

	return &source, nil
}

// updateHeaderSecret 更新Header密钥
func (s *CMDBExternalSourceService) updateHeaderSecret(secretID, value, userID string) error {
	encryptedValue, err := crypto.EncryptValue(value)
	if err != nil {
		return fmt.Errorf("failed to encrypt header value: %w", err)
	}

	return s.db.Model(&models.Secret{}).
		Where("secret_id = ?", secretID).
		Updates(map[string]interface{}{
			"value_hash": encryptedValue,
			"updated_by": userID,
			"updated_at": time.Now(),
		}).Error
}

// DeleteExternalSource 删除外部数据源
func (s *CMDBExternalSourceService) DeleteExternalSource(ctx context.Context, sourceID string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		// 1. 删除关联的secrets
		if err := tx.Where("resource_type = ? AND resource_id = ?", "cmdb_external_source", sourceID).
			Delete(&models.Secret{}).Error; err != nil {
			return fmt.Errorf("failed to delete secrets: %w", err)
		}

		// 2. 删除同步的资源索引
		if err := tx.Where("external_source_id = ?", sourceID).
			Delete(&models.ResourceIndex{}).Error; err != nil {
			return fmt.Errorf("failed to delete resource index: %w", err)
		}

		// 3. 删除同步日志（会通过外键级联删除）
		// 4. 删除数据源
		if err := tx.Where("source_id = ?", sourceID).
			Delete(&models.CMDBExternalSource{}).Error; err != nil {
			return fmt.Errorf("failed to delete external source: %w", err)
		}

		return nil
	})
}

// GetExternalSource 获取外部数据源详情
func (s *CMDBExternalSourceService) GetExternalSource(ctx context.Context, sourceID string) (*models.CMDBExternalSource, error) {
	var source models.CMDBExternalSource
	if err := s.db.Where("source_id = ?", sourceID).First(&source).Error; err != nil {
		return nil, fmt.Errorf("external source not found: %w", err)
	}
	return &source, nil
}

// ListExternalSources 列出外部数据源
func (s *CMDBExternalSourceService) ListExternalSources(ctx context.Context) ([]models.CMDBExternalSource, error) {
	var sources []models.CMDBExternalSource
	if err := s.db.Order("created_at DESC").Find(&sources).Error; err != nil {
		return nil, err
	}
	return sources, nil
}

// TestConnection 测试连接
func (s *CMDBExternalSourceService) TestConnection(ctx context.Context, sourceID string) (*models.TestConnectionResponse, error) {
	// 1. 获取数据源配置
	source, err := s.GetExternalSource(ctx, sourceID)
	if err != nil {
		return nil, err
	}

	// 2. 构建HTTP请求
	req, err := s.buildHTTPRequest(ctx, source)
	if err != nil {
		return &models.TestConnectionResponse{
			Success: false,
			Message: fmt.Sprintf("Failed to build request: %v", err),
		}, nil
	}

	// 3. 发送请求
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return &models.TestConnectionResponse{
			Success: false,
			Message: fmt.Sprintf("Connection failed: %v", err),
		}, nil
	}
	defer resp.Body.Close()

	// 4. 检查响应状态
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return &models.TestConnectionResponse{
			Success: false,
			Message: fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(body)),
		}, nil
	}

	// 5. 解析响应
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return &models.TestConnectionResponse{
			Success: false,
			Message: fmt.Sprintf("Failed to read response: %v", err),
		}, nil
	}

	var responseData interface{}
	if err := json.Unmarshal(body, &responseData); err != nil {
		return &models.TestConnectionResponse{
			Success: false,
			Message: fmt.Sprintf("Failed to parse JSON response: %v", err),
		}, nil
	}

	// 6. 提取数据
	data, err := s.extractDataFromResponse(responseData, source.ResponsePath)
	if err != nil {
		return &models.TestConnectionResponse{
			Success: false,
			Message: fmt.Sprintf("Failed to extract data: %v", err),
		}, nil
	}

	// 7. 返回样本数据
	dataArray, ok := data.([]interface{})
	if !ok {
		return &models.TestConnectionResponse{
			Success:     true,
			Message:     "Connection successful, but response is not an array",
			SampleCount: 1,
			SampleData:  []interface{}{data},
		}, nil
	}

	sampleCount := len(dataArray)
	sampleData := dataArray
	if sampleCount > 5 {
		sampleData = dataArray[:5]
	}

	return &models.TestConnectionResponse{
		Success:     true,
		Message:     fmt.Sprintf("Connection successful, found %d resources", sampleCount),
		SampleCount: sampleCount,
		SampleData:  sampleData,
	}, nil
}

// buildHTTPRequest 构建HTTP请求
func (s *CMDBExternalSourceService) buildHTTPRequest(ctx context.Context, source *models.CMDBExternalSource) (*http.Request, error) {
	var body io.Reader
	if source.HTTPMethod == "POST" && source.RequestBody != "" {
		body = bytes.NewBufferString(source.RequestBody)
	}

	req, err := http.NewRequestWithContext(ctx, source.HTTPMethod, source.APIEndpoint, body)
	if err != nil {
		return nil, err
	}

	// 添加默认headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	// 添加认证headers
	headers, err := source.GetAuthHeaders()
	if err != nil {
		return nil, fmt.Errorf("failed to get auth headers: %w", err)
	}

	for _, h := range headers {
		if h.SecretID != "" {
			// 从secrets表获取并解密value
			value, err := s.getHeaderSecretValue(h.SecretID)
			if err != nil {
				return nil, fmt.Errorf("failed to get header value for %s: %w", h.Key, err)
			}
			req.Header.Set(h.Key, value)
		}
	}

	return req, nil
}

// getHeaderSecretValue 获取Header密钥值
func (s *CMDBExternalSourceService) getHeaderSecretValue(secretID string) (string, error) {
	var secret models.Secret
	if err := s.db.Where("secret_id = ? AND is_active = true", secretID).First(&secret).Error; err != nil {
		return "", fmt.Errorf("secret not found: %w", err)
	}

	// 解密value
	value, err := crypto.DecryptValue(secret.ValueHash)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt value: %w", err)
	}

	// 更新最后使用时间
	now := time.Now()
	s.db.Model(&secret).Update("last_used_at", now)

	return value, nil
}

// extractDataFromResponse 从响应中提取数据（简化的JSONPath实现）
func (s *CMDBExternalSourceService) extractDataFromResponse(data interface{}, responsePath string) (interface{}, error) {
	if responsePath == "" {
		return data, nil
	}

	// 简化的JSONPath解析：支持 $.field.subfield 格式
	path := strings.TrimPrefix(responsePath, "$.")
	if path == "" || path == "$" {
		return data, nil
	}

	parts := strings.Split(path, ".")
	current := data

	for _, part := range parts {
		if part == "" {
			continue
		}

		switch v := current.(type) {
		case map[string]interface{}:
			var ok bool
			current, ok = v[part]
			if !ok {
				return nil, fmt.Errorf("field not found: %s", part)
			}
		default:
			return nil, fmt.Errorf("cannot access field %s on non-object", part)
		}
	}

	return current, nil
}

// SyncExternalSource 同步外部数据源
func (s *CMDBExternalSourceService) SyncExternalSource(ctx context.Context, sourceID string) error {
	// 1. 获取数据源配置
	source, err := s.GetExternalSource(ctx, sourceID)
	if err != nil {
		return err
	}

	// 2. 创建同步日志
	syncLog := &models.CMDBSyncLog{
		SourceID:  sourceID,
		StartedAt: time.Now(),
		Status:    models.SyncStatusRunning,
	}
	if err := s.db.Create(syncLog).Error; err != nil {
		return fmt.Errorf("failed to create sync log: %w", err)
	}

	// 3. 更新数据源状态
	s.db.Model(source).Updates(map[string]interface{}{
		"last_sync_status": models.SyncStatusRunning,
		"last_sync_at":     time.Now(),
	})

	// 4. 执行同步
	added, updated, deleted, syncErr := s.doSync(ctx, source)

	// 5. 更新同步日志
	now := time.Now()
	syncLog.CompletedAt = &now
	syncLog.ResourcesSynced = added + updated
	syncLog.ResourcesAdded = added
	syncLog.ResourcesUpdated = updated
	syncLog.ResourcesDeleted = deleted

	if syncErr != nil {
		syncLog.Status = models.SyncStatusFailed
		syncLog.ErrorMessage = syncErr.Error()
		s.db.Model(source).Updates(map[string]interface{}{
			"last_sync_status":  models.SyncStatusFailed,
			"last_sync_message": syncErr.Error(),
		})
	} else {
		syncLog.Status = models.SyncStatusSuccess
		s.db.Model(source).Updates(map[string]interface{}{
			"last_sync_status":  models.SyncStatusSuccess,
			"last_sync_message": fmt.Sprintf("Synced %d resources", added+updated),
			"last_sync_count":   added + updated,
		})

		// 6. 同步成功后，为外部数据源的资源创建 embedding 任务
		s.createEmbeddingTasksForExternalSource(sourceID)
	}

	s.db.Save(syncLog)

	return syncErr
}

// createEmbeddingTasksForExternalSource 为外部数据源的资源创建 embedding 任务
func (s *CMDBExternalSourceService) createEmbeddingTasksForExternalSource(sourceID string) {
	// 获取该数据源没有 embedding 的资源
	var resources []models.ResourceIndex
	s.db.Where("external_source_id = ? AND embedding IS NULL", sourceID).Find(&resources)

	if len(resources) == 0 {
		return
	}

	// 批量创建 embedding 任务
	now := time.Now()
	tasks := make([]models.EmbeddingTask, 0, len(resources))
	for _, r := range resources {
		tasks = append(tasks, models.EmbeddingTask{
			ResourceID:  r.ID,
			WorkspaceID: ExternalWorkspaceID, // 使用外部数据源的特殊 workspace_id
			Status:      models.EmbeddingTaskStatusPending,
			CreatedAt:   now,
			UpdatedAt:   now,
		})
	}

	// 批量插入，忽略重复
	s.db.Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(&tasks, 1000)

	log.Printf("[CMDBExternalSource] Created %d embedding tasks for source %s", len(tasks), sourceID)
}

// doSync 执行同步
func (s *CMDBExternalSourceService) doSync(ctx context.Context, source *models.CMDBExternalSource) (added, updated, deleted int, err error) {
	// 1. 构建HTTP请求
	req, err := s.buildHTTPRequest(ctx, source)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("failed to build request: %w", err)
	}

	// 2. 发送请求
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return 0, 0, 0, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	// 3. 解析响应
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("failed to read response: %w", err)
	}

	var responseData interface{}
	if err := json.Unmarshal(body, &responseData); err != nil {
		return 0, 0, 0, fmt.Errorf("failed to parse JSON: %w", err)
	}

	// 4. 提取数据
	data, err := s.extractDataFromResponse(responseData, source.ResponsePath)
	if err != nil {
		return 0, 0, 0, err
	}

	// 确保data是数组类型
	var dataArray []interface{}
	switch v := data.(type) {
	case []interface{}:
		dataArray = v
	case []map[string]interface{}:
		// 转换为[]interface{}
		for _, item := range v {
			dataArray = append(dataArray, item)
		}
	default:
		// 如果不是数组，包装成单元素数组
		dataArray = []interface{}{data}
	}

	log.Printf("[CMDBSync] Extracted data: type=%T, dataArray length=%d", data, len(dataArray))

	// 5. 获取字段映射
	var fieldMapping map[string]string
	if err := json.Unmarshal(source.FieldMapping, &fieldMapping); err != nil {
		return 0, 0, 0, fmt.Errorf("failed to parse field_mapping: %w", err)
	}

	// 6. 获取现有资源（用于增量更新）
	existingResources := make(map[string]*models.ResourceIndex)
	var existing []models.ResourceIndex
	s.db.Where("external_source_id = ?", source.SourceID).Find(&existing)
	for i := range existing {
		existingResources[existing[i].PrimaryKeyValue] = &existing[i]
	}

	// 7. 处理每个资源
	processedKeys := make(map[string]bool)
	for _, item := range dataArray {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		// 提取主键值
		primaryKeyValue := s.extractFieldValue(itemMap, source.PrimaryKeyField)
		if primaryKeyValue == "" {
			continue
		}
		processedKeys[primaryKeyValue] = true

		// 构建资源索引记录
		record := s.buildResourceIndexFromItem(source, itemMap, fieldMapping, primaryKeyValue)

		// 检查是否已存在
		if existingRecord, exists := existingResources[primaryKeyValue]; exists {
			// 更新现有记录 - 使用 Updates 而不是 Save，避免覆盖 embedding 相关字段
			if err := s.db.Model(&models.ResourceIndex{}).
				Where("id = ?", existingRecord.ID).
				Updates(map[string]interface{}{
					"terraform_address":   record.TerraformAddress,
					"resource_type":       record.ResourceType,
					"resource_name":       record.ResourceName,
					"cloud_resource_id":   record.CloudResourceID,
					"cloud_resource_name": record.CloudResourceName,
					"cloud_resource_arn":  record.CloudResourceARN,
					"description":         record.Description,
					"tags":                record.Tags,
					"attributes":          record.Attributes,
					"cloud_provider":      record.CloudProvider,
					"cloud_account_id":    record.CloudAccountID,
					"cloud_account_name":  record.CloudAccountName,
					"cloud_region":        record.CloudRegion,
					"primary_key_value":   record.PrimaryKeyValue,
					"last_synced_at":      record.LastSyncedAt,
					// 注意：不更新 embedding, embedding_text, embedding_model, embedding_updated_at 字段
					// 这样可以保留已生成的 embedding 数据
				}).Error; err == nil {
				updated++
			}
		} else {
			// 创建新记录
			if err := s.db.Create(&record).Error; err == nil {
				added++
			}
		}
	}

	// 8. 删除不再存在的资源
	for key, existingRecord := range existingResources {
		if !processedKeys[key] {
			if err := s.db.Delete(existingRecord).Error; err == nil {
				deleted++
			}
		}
	}

	return added, updated, deleted, nil
}

// extractFieldValue 从数据中提取字段值（简化的JSONPath实现）
func (s *CMDBExternalSourceService) extractFieldValue(data map[string]interface{}, path string) string {
	if path == "" {
		return ""
	}

	// 简化的JSONPath解析：支持 $.field.subfield 格式
	path = strings.TrimPrefix(path, "$.")
	if path == "" || path == "$" {
		return ""
	}

	parts := strings.Split(path, ".")
	var current interface{} = data

	for _, part := range parts {
		if part == "" {
			continue
		}

		switch v := current.(type) {
		case map[string]interface{}:
			var ok bool
			current, ok = v[part]
			if !ok {
				return ""
			}
		default:
			return ""
		}
	}

	switch v := current.(type) {
	case string:
		return v
	case float64:
		return fmt.Sprintf("%v", v)
	case int:
		return fmt.Sprintf("%d", v)
	case bool:
		return fmt.Sprintf("%v", v)
	default:
		if v != nil {
			return fmt.Sprintf("%v", v)
		}
		return ""
	}
}

// ExternalWorkspaceID 外部数据源使用的特殊workspace_id
// 使用双下划线前缀避免与真实workspace冲突
const ExternalWorkspaceID = "__external__"

// buildResourceIndexFromItem 从数据项构建资源索引记录
func (s *CMDBExternalSourceService) buildResourceIndexFromItem(source *models.CMDBExternalSource, item map[string]interface{}, fieldMapping map[string]string, primaryKeyValue string) models.ResourceIndex {
	// 为外部数据源生成唯一的terraform_address
	// 格式: external.{source_id}.{primary_key_value}
	terraformAddress := fmt.Sprintf("external.%s.%s", source.SourceID, primaryKeyValue)

	record := models.ResourceIndex{
		WorkspaceID:      ExternalWorkspaceID, // 外部数据源使用特殊的workspace_id，避免与真实workspace冲突
		TerraformAddress: terraformAddress,
		SourceType:       "external",
		ExternalSourceID: source.SourceID,
		CloudProvider:    source.CloudProvider,
		CloudAccountID:   source.AccountID,
		CloudAccountName: source.AccountName,
		CloudRegion:      source.Region,
		PrimaryKeyValue:  primaryKeyValue,
		ResourceMode:     "managed",
		LastSyncedAt:     time.Now(),
	}

	// 从字段映射提取值
	if path, ok := fieldMapping["resource_type"]; ok {
		record.ResourceType = s.extractFieldValue(item, path)
	}
	if record.ResourceType == "" && source.ResourceTypeFilter != "" {
		record.ResourceType = source.ResourceTypeFilter
	}

	if path, ok := fieldMapping["resource_name"]; ok {
		record.ResourceName = s.extractFieldValue(item, path)
	}

	if path, ok := fieldMapping["cloud_resource_id"]; ok {
		record.CloudResourceID = s.extractFieldValue(item, path)
	}

	if path, ok := fieldMapping["cloud_resource_name"]; ok {
		record.CloudResourceName = s.extractFieldValue(item, path)
	}

	if path, ok := fieldMapping["cloud_resource_arn"]; ok {
		record.CloudResourceARN = s.extractFieldValue(item, path)
	}

	if path, ok := fieldMapping["description"]; ok {
		record.Description = s.extractFieldValue(item, path)
	}

	// 提取tags
	if path, ok := fieldMapping["tags"]; ok {
		tagsData, _ := s.extractDataFromResponse(item, path)
		if tagsData != nil {
			if tagsJSON, err := json.Marshal(tagsData); err == nil {
				record.Tags = tagsJSON
			}
		}
	}

	// 提取attributes
	if path, ok := fieldMapping["attributes"]; ok {
		attrsData, _ := s.extractDataFromResponse(item, path)
		if attrsData != nil {
			if attrsJSON, err := json.Marshal(attrsData); err == nil {
				record.Attributes = attrsJSON
			}
		}
	}

	return record
}

// GetSyncLogs 获取同步日志
func (s *CMDBExternalSourceService) GetSyncLogs(ctx context.Context, sourceID string, limit int) ([]models.CMDBSyncLog, error) {
	if limit <= 0 {
		limit = 20
	}

	var logs []models.CMDBSyncLog
	if err := s.db.Where("source_id = ?", sourceID).
		Order("started_at DESC").
		Limit(limit).
		Find(&logs).Error; err != nil {
		return nil, err
	}

	return logs, nil
}
