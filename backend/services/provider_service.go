package services

import (
	"encoding/json"
	"fmt"
	"iac-platform/internal/models"
)

// ProviderService Provider配置服务
type ProviderService struct{}

// NewProviderService 创建Provider服务
func NewProviderService() *ProviderService {
	return &ProviderService{}
}

// ValidateProviderConfig 验证Provider配置
func (s *ProviderService) ValidateProviderConfig(config map[string]interface{}) error {
	providers, ok := config["provider"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid provider config structure: 'provider' field must be an object")
	}

	// 检查每个Provider类型
	for providerType, providerList := range providers {
		list, ok := providerList.([]interface{})
		if !ok {
			return fmt.Errorf("invalid provider list for %s: must be an array", providerType)
		}

		// 检查alias唯一性
		aliases := make(map[string]bool)
		hasDefault := false

		for i, p := range list {
			provider, ok := p.(map[string]interface{})
			if !ok {
				return fmt.Errorf("invalid provider config at index %d for %s", i, providerType)
			}

			alias, hasAlias := provider["alias"].(string)

			if !hasAlias || alias == "" {
				// 没有alias的是默认provider
				if hasDefault {
					return fmt.Errorf("multiple default providers for %s (only one allowed)", providerType)
				}
				hasDefault = true
			} else {
				// 检查alias唯一性
				if aliases[alias] {
					return fmt.Errorf("duplicate alias '%s' for provider %s", alias, providerType)
				}
				aliases[alias] = true
			}

			// 验证必需字段
			if err := s.validateProviderFields(providerType, provider); err != nil {
				return fmt.Errorf("validation failed for %s provider: %w", providerType, err)
			}
		}
	}

	return nil
}

// validateProviderFields 验证Provider必需字段
func (s *ProviderService) validateProviderFields(providerType string, config map[string]interface{}) error {
	switch providerType {
	case "aws":
		// AWS Provider必需region
		if _, ok := config["region"]; !ok {
			return fmt.Errorf("region is required for AWS provider")
		}

		// 如果使用AKSK方式，检查access_key和secret_key
		if accessKey, hasAccessKey := config["access_key"]; hasAccessKey {
			if accessKey == "" {
				return fmt.Errorf("access_key cannot be empty")
			}
			if _, hasSecretKey := config["secret_key"]; !hasSecretKey {
				return fmt.Errorf("secret_key is required when access_key is provided")
			}
		}

		// 如果使用assume_role方式，检查role_arn
		if assumeRole, hasAssumeRole := config["assume_role"]; hasAssumeRole {
			if roleList, ok := assumeRole.([]interface{}); ok && len(roleList) > 0 {
				if role, ok := roleList[0].(map[string]interface{}); ok {
					if roleArn, ok := role["role_arn"].(string); !ok || roleArn == "" {
						return fmt.Errorf("role_arn is required in assume_role configuration")
					}
				}
			}
		}

	case "azure":
		// Azure Provider验证（未来实现）
		return fmt.Errorf("azure provider is not yet supported")

	case "google":
		// Google Cloud Provider验证（未来实现）
		return fmt.Errorf("google provider is not yet supported")

	case "alicloud":
		// Alibaba Cloud Provider验证（未来实现）
		return fmt.Errorf("alicloud provider is not yet supported")
	}

	return nil
}

// FilterSensitiveInfo 过滤敏感信息（用于API响应）
func (s *ProviderService) FilterSensitiveInfo(config map[string]interface{}) map[string]interface{} {
	// 深拷贝
	filtered := s.deepCopy(config)

	// 遍历所有provider
	if providers, ok := filtered["provider"].(map[string]interface{}); ok {
		for providerType, providerList := range providers {
			if list, ok := providerList.([]interface{}); ok {
				for i, p := range list {
					if provider, ok := p.(map[string]interface{}); ok {
						// 隐藏敏感字段
						if _, exists := provider["access_key"]; exists {
							provider["access_key"] = "***HIDDEN***"
						}
						if _, exists := provider["secret_key"]; exists {
							provider["secret_key"] = "***HIDDEN***"
						}

						// 隐藏assume_role中的敏感信息（如果有）
						if assumeRole, ok := provider["assume_role"].([]interface{}); ok {
							for j, role := range assumeRole {
								if roleMap, ok := role.(map[string]interface{}); ok {
									if _, exists := roleMap["external_id"]; exists {
										roleMap["external_id"] = "***HIDDEN***"
									}
									assumeRole[j] = roleMap
								}
							}
							provider["assume_role"] = assumeRole
						}

						list[i] = provider
					}
				}
				providers[providerType] = list
			}
		}
	}

	return filtered
}

// deepCopy 深拷贝map
func (s *ProviderService) deepCopy(src map[string]interface{}) map[string]interface{} {
	// 使用JSON序列化/反序列化实现深拷贝
	data, err := json.Marshal(src)
	if err != nil {
		return make(map[string]interface{})
	}

	var dst map[string]interface{}
	if err := json.Unmarshal(data, &dst); err != nil {
		return make(map[string]interface{})
	}

	return dst
}

// BuildProviderTFJSON 构建provider.tf.json内容
func (s *ProviderService) BuildProviderTFJSON(workspace *models.Workspace) (map[string]interface{}, error) {
	if workspace.ProviderConfig == nil {
		return nil, fmt.Errorf("provider_config is required")
	}

	// 验证配置
	if err := s.ValidateProviderConfig(workspace.ProviderConfig); err != nil {
		return nil, fmt.Errorf("invalid provider config: %w", err)
	}

	// 直接返回provider_config，它已经是正确的Terraform JSON格式
	return workspace.ProviderConfig, nil
}

// GetProviderConfigForDisplay 获取用于显示的Provider配置（隐藏敏感信息）
func (s *ProviderService) GetProviderConfigForDisplay(workspace *models.Workspace) map[string]interface{} {
	if workspace.ProviderConfig == nil {
		return make(map[string]interface{})
	}

	return s.FilterSensitiveInfo(workspace.ProviderConfig)
}

// TestProviderConnection 测试Provider连接（未来实现）
func (s *ProviderService) TestProviderConnection(
	providerType string,
	config map[string]interface{},
) (bool, string, error) {
	// TODO: 实现实际的连接测试
	// 这需要根据不同的Provider类型调用相应的SDK进行验证

	switch providerType {
	case "aws":
		// TODO: 使用AWS SDK测试连接
		// 例如：调用STS GetCallerIdentity验证凭证
		return true, "AWS connection test not yet implemented", nil

	case "azure":
		return false, "Azure provider not yet supported", fmt.Errorf("not implemented")

	case "google":
		return false, "Google Cloud provider not yet supported", fmt.Errorf("not implemented")

	default:
		return false, fmt.Sprintf("Unknown provider type: %s", providerType), fmt.Errorf("unknown provider")
	}
}
