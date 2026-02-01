# Workspace模块 - Provider配置详细设计

> **文档版本**: v1.0  
> **创建日期**: 2025-10-11  
> **状态**: 完整设计  
> **相关文档**: [13-workspace-settings-design.md](./13-workspace-settings-design.md), [15-terraform-execution-detail.md](./15-terraform-execution-detail.md)

## 📘 概述

本文档详细定义Workspace的Provider配置功能，包括Provider认证方式、版本约束、高级参数配置，以及在Terraform执行时如何生成provider.tf.json文件。

## 🎯 核心需求

### 1. Provider配置存储
- 存储在`workspaces.provider_config`字段（JSONB类型）
- 支持多个Provider同时配置
- 支持同一Provider的多个配置（通过alias区分）

### 2. Provider配置结构
```json
{
  "provider": {
    "aws": [
      {
        "alias": "us-east",
        "region": "us-east-1",
        "access_key": "AKIAIOSFODNN7EXAMPLE",
        "secret_key": "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
      },
      {
        "alias": "us-west",
        "region": "us-west-2",
        "assume_role": [
          {
            "role_arn": "arn:aws:iam::123456789012:role/TerraformRole"
          }
        ]
      }
    ]
  },
  "terraform": [
    {
      "required_providers": [
        {
          "aws": {
            "source": "hashicorp/aws",
            "version": "~> 6.0"
          }
        }
      ]
    }
  ]
}
```

### 3. 支持的认证方式

#### AWS Provider
1. **AKSK方式** (Access Key / Secret Key)
2. **IAM Role方式** (使用EC2实例角色)
3. **Assume Role方式** (跨账号访问)

#### 其他Provider（未来扩展）
- Azure: Service Principal
- GCP: Service Account
- 阿里云: AccessKey

## 🎨 UI设计

### Settings页面新增Provider子页面

#### 导航结构
```
Settings
├── General
├── Locking
├── Provider          ← 新增
├── Notifications
└── Destruction and Deletion
```

### Provider配置页面布局

```tsx
<div className={styles.providerContainer}>
  {/* 页面标题 */}
  <div className={styles.pageHeader}>
    <h2 className={styles.pageTitle}>Provider Configuration</h2>
    <p className={styles.pageDescription}>
      Configure Terraform providers and their authentication methods. 
      These settings will be used to generate provider.tf.json during execution.
    </p>
  </div>

  {/* Provider列表 */}
  <div className={styles.providerList}>
    {providers.map((provider, index) => (
      <ProviderCard
        key={index}
        provider={provider}
        onEdit={() => handleEdit(index)}
        onDelete={() => handleDelete(index)}
      />
    ))}
  </div>

  {/* 添加Provider按钮 */}
  <button onClick={handleAddProvider} className={styles.addButton}>
    + Add Provider
  </button>

  {/* 保存按钮 */}
  <div className={styles.actions}>
    <button 
      onClick={handleSave} 
      className={styles.saveButton}
      disabled={!hasChanges}
    >
      Save Settings
    </button>
    {hasChanges && (
      <span className={styles.unsavedHint}>You have unsaved changes</span>
    )}
  </div>
</div>
```

### Provider卡片设计

```tsx
const ProviderCard: React.FC<ProviderCardProps> = ({ provider, onEdit, onDelete }) => {
  return (
    <div className={styles.providerCard}>
      {/* 卡片头部 */}
      <div className={styles.cardHeader}>
        <div className={styles.cardTitle}>
          <span className={styles.providerIcon}>☁️</span>
          <span className={styles.providerName}>{provider.type}</span>
          {provider.alias && (
            <span className={styles.aliasBadge}>{provider.alias}</span>
          )}
        </div>
        <div className={styles.cardActions}>
          <button onClick={onEdit} className={styles.editButton}>
            Edit
          </button>
          <button onClick={onDelete} className={styles.deleteButton}>
            Delete
          </button>
        </div>
      </div>

      {/* 卡片内容 */}
      <div className={styles.cardContent}>
        {/* 认证方式 */}
        <div className={styles.configRow}>
          <span className={styles.configLabel}>Authentication:</span>
          <span className={styles.configValue}>{provider.authMethod}</span>
        </div>

        {/* Region */}
        {provider.region && (
          <div className={styles.configRow}>
            <span className={styles.configLabel}>Region:</span>
            <span className={styles.configValue}>{provider.region}</span>
          </div>
        )}

        {/* 版本约束 */}
        {provider.version && (
          <div className={styles.configRow}>
            <span className={styles.configLabel}>Version:</span>
            <span className={styles.configValue}>
              {provider.versionConstraint} {provider.version}
            </span>
          </div>
        )}

        {/* 高级参数 */}
        {provider.advancedParams && Object.keys(provider.advancedParams).length > 0 && (
          <div className={styles.configRow}>
            <span className={styles.configLabel}>Advanced:</span>
            <span className={styles.configValue}>
              {Object.keys(provider.advancedParams).length} parameters
            </span>
          </div>
        )}
      </div>
    </div>
  );
};
```

### Provider编辑表单

```tsx
const ProviderForm: React.FC<ProviderFormProps> = ({ 
  provider, 
  onSave, 
  onCancel 
}) => {
  const [formData, setFormData] = useState({
    type: provider?.type || 'aws',
    alias: provider?.alias || '',
    authMethod: provider?.authMethod || 'iam_role',
    region: provider?.region || '',
    // AKSK方式
    accessKey: provider?.accessKey || '',
    secretKey: provider?.secretKey || '',
    // Assume Role方式
    roleArn: provider?.roleArn || '',
    // 版本约束
    versionConstraint: provider?.versionConstraint || '~>',
    version: provider?.version || '',
    // 高级参数
    advancedParams: provider?.advancedParams || {}
  });

  return (
    <form onSubmit={handleSubmit} className={styles.providerForm}>
      {/* Provider类型 */}
      <div className={styles.formSection}>
        <h4 className={styles.formSectionTitle}>Provider Type</h4>
        <select
          value={formData.type}
          onChange={(e) => setFormData({ ...formData, type: e.target.value })}
          className={styles.select}
        >
          <option value="aws">AWS</option>
          <option value="azure">Azure (Coming Soon)</option>
          <option value="google">Google Cloud (Coming Soon)</option>
          <option value="alicloud">Alibaba Cloud (Coming Soon)</option>
        </select>
      </div>

      {/* Alias（可选） */}
      <div className={styles.formSection}>
        <h4 className={styles.formSectionTitle}>Alias (Optional)</h4>
        <input
          type="text"
          value={formData.alias}
          onChange={(e) => setFormData({ ...formData, alias: e.target.value })}
          className={styles.input}
          placeholder="e.g., us-east, production"
        />
        <div className={styles.hint}>
          Use alias when configuring multiple instances of the same provider. 
          Leave empty for default provider.
        </div>
      </div>

      {/* 认证方式 */}
      <div className={styles.formSection}>
        <h4 className={styles.formSectionTitle}>Authentication Method</h4>
        <div className={styles.radioGroup}>
          <label className={styles.radioLabel}>
            <input
              type="radio"
              value="iam_role"
              checked={formData.authMethod === 'iam_role'}
              onChange={(e) => setFormData({ ...formData, authMethod: e.target.value })}
            />
            <div>
              <strong>IAM Role</strong>
              <p>Use IAM role attached to EC2 instance or ECS task (recommended)</p>
            </div>
          </label>

          <label className={styles.radioLabel}>
            <input
              type="radio"
              value="aksk"
              checked={formData.authMethod === 'aksk'}
              onChange={(e) => setFormData({ ...formData, authMethod: e.target.value })}
            />
            <div>
              <strong>Access Key / Secret Key</strong>
              <p>Use static credentials (not recommended for production)</p>
            </div>
          </label>

          <label className={styles.radioLabel}>
            <input
              type="radio"
              value="assume_role"
              checked={formData.authMethod === 'assume_role'}
              onChange={(e) => setFormData({ ...formData, authMethod: e.target.value })}
            />
            <div>
              <strong>Assume Role</strong>
              <p>Assume a role in another AWS account</p>
            </div>
          </label>
        </div>
      </div>

      {/* Region配置 */}
      <div className={styles.formSection}>
        <h4 className={styles.formSectionTitle}>Region *</h4>
        <input
          type="text"
          value={formData.region}
          onChange={(e) => setFormData({ ...formData, region: e.target.value })}
          className={styles.input}
          placeholder="e.g., us-east-1, ap-northeast-1"
          required
        />
      </div>

      {/* AKSK方式的字段 */}
      {formData.authMethod === 'aksk' && (
        <>
          <div className={styles.formSection}>
            <h4 className={styles.formSectionTitle}>Access Key *</h4>
            <input
              type="text"
              value={formData.accessKey}
              onChange={(e) => setFormData({ ...formData, accessKey: e.target.value })}
              className={styles.input}
              placeholder="AKIAIOSFODNN7EXAMPLE"
              required
            />
          </div>

          <div className={styles.formSection}>
            <h4 className={styles.formSectionTitle}>Secret Key *</h4>
            <input
              type="password"
              value={formData.secretKey}
              onChange={(e) => setFormData({ ...formData, secretKey: e.target.value })}
              className={styles.input}
              placeholder="wJalrXUtnFEMI/K7MDENG/bPxRfiCY..."
              required
            />
            <div className={styles.warning}>
              <span className={styles.warningIcon}></span>
              <span>Secret key will be stored in database. Consider using IAM role instead.</span>
            </div>
          </div>
        </>
      )}

      {/* Assume Role方式的字段 */}
      {formData.authMethod === 'assume_role' && (
        <div className={styles.formSection}>
          <h4 className={styles.formSectionTitle}>Role ARN *</h4>
          <input
            type="text"
            value={formData.roleArn}
            onChange={(e) => setFormData({ ...formData, roleArn: e.target.value })}
            className={styles.input}
            placeholder="arn:aws:iam::123456789012:role/TerraformRole"
            required
          />
          <div className={styles.hint}>
            The IAM role to assume for this provider configuration
          </div>
        </div>
      )}

      {/* 版本约束 */}
      <div className={styles.formSection}>
        <h4 className={styles.formSectionTitle}>Version Constraint</h4>
        <div className={styles.versionConstraint}>
          <select
            value={formData.versionConstraint}
            onChange={(e) => setFormData({ ...formData, versionConstraint: e.target.value })}
            className={styles.constraintSelect}
          >
            <option value="~>">~> (Pessimistic)</option>
            <option value=">=">&gt;= (Greater or equal)</option>
            <option value=">">&gt; (Greater than)</option>
            <option value="=">=  (Exact)</option>
            <option value="<=">&lt;= (Less or equal)</option>
            <option value="<">&lt; (Less than)</option>
          </select>
          <input
            type="text"
            value={formData.version}
            onChange={(e) => setFormData({ ...formData, version: e.target.value })}
            className={styles.versionInput}
            placeholder="6.0"
          />
        </div>
        <div className={styles.hint}>
          Example: ~> 6.0 means &gt;= 6.0.0 and &lt; 7.0.0
        </div>
      </div>

      {/* 高级参数 */}
      <div className={styles.formSection}>
        <h4 className={styles.formSectionTitle}>Advanced Parameters (Optional)</h4>
        <div className={styles.advancedParams}>
          {Object.entries(formData.advancedParams).map(([key, value], index) => (
            <div key={index} className={styles.paramRow}>
              <input
                type="text"
                value={key}
                onChange={(e) => handleParamKeyChange(index, e.target.value)}
                className={styles.paramKey}
                placeholder="parameter name"
              />
              <textarea
                value={typeof value === 'object' ? JSON.stringify(value, null, 2) : String(value)}
                onChange={(e) => handleParamValueChange(index, e.target.value)}
                className={styles.paramValue}
                placeholder="value (string, number, boolean, or JSON)"
                rows={2}
              />
              <button
                type="button"
                onClick={() => handleRemoveParam(index)}
                className={styles.removeParamButton}
              >
                ×
              </button>
            </div>
          ))}
          <button
            type="button"
            onClick={handleAddParam}
            className={styles.addParamButton}
          >
            + Add Parameter
          </button>
        </div>
        <div className={styles.hint}>
          Add any provider-specific parameters. Values can be strings, numbers, booleans, or JSON objects.
        </div>
      </div>

      {/* 表单操作 */}
      <div className={styles.formActions}>
        <button type="submit" className={styles.primaryButton}>
          {provider ? 'Update Provider' : 'Add Provider'}
        </button>
        <button type="button" onClick={onCancel} className={styles.cancelButton}>
          Cancel
        </button>
      </div>
    </form>
  );
};
```

## 📊 数据结构设计

### TypeScript接口定义

```typescript
// Provider配置接口
interface ProviderConfig {
  type: string;                    // aws, azure, google, alicloud
  alias?: string;                  // 别名（可选）
  authMethod: 'iam_role' | 'aksk' | 'assume_role';
  region: string;                  // 区域
  
  // AKSK方式
  accessKey?: string;
  secretKey?: string;
  
  // Assume Role方式
  assumeRole?: {
    roleArn: string;
    sessionName?: string;
    externalId?: string;
  };
  
  // 版本约束
  versionConstraint?: '~>' | '>=' | '>' | '=' | '<=' | '<';
  version?: string;
  
  // 高级参数
  advancedParams?: Record<string, any>;
}

// Workspace Provider配置
interface WorkspaceProviderConfig {
  provider: {
    [providerType: string]: ProviderConfig[];
  };
  terraform: Array<{
    required_providers: Array<{
      [providerType: string]: {
        source: string;
        version: string;
      };
    }>;
  }>;
}
```

### 数据库存储格式

```json
{
  "provider": {
    "aws": [
      {
        "alias": "us-east",
        "region": "us-east-1",
        "access_key": "AKIAIOSFODNN7EXAMPLE",
        "secret_key": "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
        "max_retries": 3,
        "skip_credentials_validation": false
      },
      {
        "alias": "us-west",
        "region": "us-west-2",
        "assume_role": [
          {
            "role_arn": "arn:aws:iam::123456789012:role/TerraformRole",
            "session_name": "terraform-session"
          }
        ]
      },
      {
        "region": "ap-northeast-1"
      }
    ]
  },
  "terraform": [
    {
      "required_providers": [
        {
          "aws": {
            "source": "hashicorp/aws",
            "version": "~> 6.0"
          }
        }
      ]
    }
  ]
}
```

## 🔧 Fetching阶段Provider配置处理

### 生成provider.tf.json文件

```go
// 在Fetching阶段生成provider.tf.json
func (s *TerraformExecutor) GenerateProviderConfig(
    workspace *models.Workspace,
    workDir string,
) error {
    // 1. 检查provider_config是否存在
    if workspace.ProviderConfig == nil {
        return fmt.Errorf("provider_config is required")
    }
    
    // 2. 直接使用workspace.ProviderConfig
    // 这个字段已经是正确的Terraform JSON格式
    providerConfig := workspace.ProviderConfig
    
    // 3. 写入provider.tf.json文件
    providerFile := filepath.Join(workDir, "provider.tf.json")
    data, err := json.MarshalIndent(providerConfig, "", "  ")
    if err != nil {
        return fmt.Errorf("failed to marshal provider config: %w", err)
    }
    
    if err := os.WriteFile(providerFile, data, 0644); err != nil {
        return fmt.Errorf("failed to write provider.tf.json: %w", err)
    }
    
    log.Printf("Generated provider.tf.json in %s", workDir)
    return nil
}
```

### provider.tf.json示例输出

#### 示例1: IAM Role方式
```json
{
  "provider": {
    "aws": [
      {
        "region": "ap-northeast-1"
      }
    ]
  },
  "terraform": [
    {
      "required_providers": [
        {
          "aws": {
            "source": "hashicorp/aws",
            "version": "~> 6.0"
          }
        }
      ]
    }
  ]
}
```

#### 示例2: AKSK方式
```json
{
  "provider": {
    "aws": [
      {
        "region": "us-east-1",
        "access_key": "AKIAIOSFODNN7EXAMPLE",
        "secret_key": "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
      }
    ]
  },
  "terraform": [
    {
      "required_providers": [
        {
          "aws": {
            "source": "hashicorp/aws",
            "version": "~> 6.0"
          }
        }
      ]
    }
  ]
}
```

#### 示例3: Assume Role方式
```json
{
  "provider": {
    "aws": [
      {
        "region": "ap-northeast-1",
        "assume_role": [
          {
            "role_arn": "arn:aws:iam::817275903355:role/ops-privileged-tfe"
          }
        ]
      }
    ]
  },
  "terraform": [
    {
      "required_providers": [
        {
          "aws": {
            "source": "hashicorp/aws",
            "version": "~> 6.0"
          }
        }
      ]
    }
  ]
}
```

#### 示例4: 多Provider配置（带alias）
```json
{
  "provider": {
    "aws": [
      {
        "alias": "us-east",
        "region": "us-east-1",
        "max_retries": 5
      },
      {
        "alias": "us-west",
        "region": "us-west-2",
        "assume_role": [
          {
            "role_arn": "arn:aws:iam::123456789012:role/CrossAccountRole"
          }
        ]
      }
    ]
  },
  "terraform": [
    {
      "required_providers": [
        {
          "aws": {
            "source": "hashicorp/aws",
            "version": "~> 6.0"
          }
        }
      ]
    }
  ]
}
```

#### 示例5: 带高级参数
```json
{
  "provider": {
    "aws": [
      {
        "region": "us-east-1",
        "max_retries": 5,
        "skip_credentials_validation": false,
        "skip_metadata_api_check": false,
        "skip_region_validation": false,
        "default_tags": [
          {
            "tags": {
              "Environment": "Production",
              "ManagedBy": "Terraform"
            }
          }
        ]
      }
    ]
  },
  "terraform": [
    {
      "required_providers": [
        {
          "aws": {
            "source": "hashicorp/aws",
            "version": "~> 6.0"
          }
        }
      ]
    }
  ]
}
```

## 🔐 安全考虑

### 1. 敏感信息处理

```go
// Provider配置响应（隐藏敏感信息）
func (s *WorkspaceService) GetProviderConfigForDisplay(
    workspace *models.Workspace,
) map[string]interface{} {
    config := workspace.ProviderConfig
    
    // 深拷贝
    displayConfig := deepCopy(config)
    
    // 遍历所有provider
    if providers, ok := displayConfig["provider"].(map[string]interface{}); ok {
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
                        list[i] = provider
                    }
                }
                providers[providerType] = list
            }
        }
    }
    
    return displayConfig
}
```

### 2. 凭证验证

```go
// 验证Provider配置
func (s *WorkspaceService) ValidateProviderConfig(
    config map[string]interface{},
) error {
    providers, ok := config["provider"].(map[string]interface{})
    if !ok {
        return fmt.Errorf("invalid provider config structure")
    }
    
    for providerType, providerList := range providers {
        list, ok := providerList.([]interface{})
        if !ok {
            return fmt.Errorf("invalid provider list for %s", providerType)
        }
        
        // 检查alias唯一性
        aliases := make(map[string]bool)
        hasDefault := false
        
        for _, p := range list {
            provider, ok := p.(map[string]interface{})
            if !ok {
                continue
            }
            
            alias, hasAlias := provider["alias"].(string)
            
            if !hasAlias {
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
            if providerType == "aws" {
                if _, ok := provider["region"]; !ok {
                    return fmt.Errorf("region is required for AWS provider")
                }
            }
        }
    }
    
    return nil
}
```

## 🔄 前端实现逻辑

### Provider配置状态管理

```typescript
const ProviderSettings: React.FC = () => {
  const { workspaceId } = useParams();
  const { showToast } = useToast();
  
  const [providers, setProviders] = useState<ProviderConfig[]>([]);
  const [terraformConfig, setTerraformConfig] = useState<any>(null);
  const [editingIndex, setEditingIndex] = useState<number | null>(null);
  const [showForm, setShowForm] = useState(false);
  const [hasChanges, setHasChanges] = useState(false);
  const [loading, setLoading] = useState(false);

  // 加载Provider配置
  useEffect(() => {
    fetchProviderConfig();
  }, [workspaceId]);

  const fetchProviderConfig = async () => {
    try {
      const response = await api.get(`/workspaces/${workspaceId}`);
      const workspace = response.data || response;
      
      if (workspace.provider_config) {
        // 解析provider配置
        const config = workspace.provider_config;
        const providerList: ProviderConfig[] = [];
        
        // 提取provider配置
        if (config.provider) {
          Object.entries(config.provider).forEach(([type, configs]: [string, any]) => {
            if (Array.isArray(configs)) {
              configs.forEach((cfg: any) => {
                providerList.push(parseProviderConfig(type, cfg));
              });
            }
          });
        }
        
        setProviders(providerList);
        setTerraformConfig(config.terraform);
      }
    } catch (error) {
      showToast(extractErrorMessage(error), 'error');
    }
  };

  // 解析Provider配置
  const parseProviderConfig = (type: string, config: any): ProviderConfig => {
    const provider: ProviderConfig = {
      type,
      alias: config.alias,
      region: config.region,
      authMethod: 'iam_role',
      advancedParams: {}
    };

    // 判断认证方式
    if (config.access_key && config.secret_key) {
      provider.authMethod = 'aksk';
      provider.accessKey = config.access_key;
      provider.secretKey = config.secret_key;
    } else if (config.assume_role) {
      provider.authMethod = 'assume_role';
      provider.assumeRole = config.assume_role[0];
    }

    // 提取高级参数
    const standardFields = ['alias', 'region', 'access_key', 'secret_key', 'assume_role'];
    Object.entries(config).forEach(([key, value]) => {
      if (!standardFields.includes(key)) {
        provider.advancedParams![key] = value;
      }
    });

    return provider;
  };

  // 构建保存数据
  const buildSaveData = (): WorkspaceProviderConfig => {
    const providerMap: Record<string, any[]> = {};
    
    // 按类型分组
    providers.forEach(p => {
      if (!providerMap[p.type]) {
        providerMap[p.type] = [];
      }
      
      const config: any = {
        region: p.region,
        ...p.advancedParams
      };
      
      if (p.alias) {
        config.alias = p.alias;
      }
      
      // 根据认证方式添加字段
      if (p.authMethod === 'aksk') {
        config.access_key = p.accessKey;
        config.secret_key = p.secretKey;
      } else if (p.authMethod === 'assume_role' && p.assumeRole) {
        config.assume_role = [p.assumeRole];
      }
      
      providerMap[p.type].push(config);
    });
    
    // 构建terraform配置
    const requiredProviders: any = {};
    providers.forEach(p => {
      if (p.version) {
        const constraint = p.versionConstraint || '~>';
        requiredProviders[p.type] = {
          source: `hashicorp/${p.type}`,
          version: `${constraint} ${p.version}`
        };
      }
    });
    
    return {
      provider: providerMap,
      terraform: Object.keys(requiredProviders).length > 0 ? [
        {
          required_providers: [requiredProviders]
        }
      ] : []
    };
  };

  // 保存配置
  const handleSave = async () => {
    try {
      setLoading(true);
      
      const providerConfig = buildSaveData();
      
      // 验证配置
      if (!validateProviderConfig(providerConfig)) {
        showToast('Provider配置验证失败', 'error');
        return;
      }
      
      // 保存到后端
      await api.patch(`/workspaces/${workspaceId}`, {
        provider_config: providerConfig
      });
      
      showToast('Provider配置已保存', 'success');
      setHasChanges(false);
    } catch (error) {
      showToast(extractErrorMessage(error), 'error');
    } finally {
      setLoading(false);
    }
  };

  // 验证配置
  const validateProviderConfig = (config: WorkspaceProviderConfig): boolean => {
    // 检查alias唯一性
    const aliases = new Set<string>();
    let hasDefault = false;
    
    Object.values(config.provider).forEach(providerList => {
      providerList.forEach(p => {
        if (p.alias) {
          if (aliases.has(p.alias)) {
            showToast(`Duplicate alias: ${p.alias}`, 'error');
            return false;
          }
          aliases.add(p.alias);
        } else {
          if (hasDefault) {
            showToast('Only one default provider allowed', 'error');
            return false;
          }
          hasDefault = true;
        }
      });
    });
    
    return true;
  };
};
```

## 🎯 版本约束说明

### 约束符号含义

| 符号 | 含义 | 示例 | 匹配版本 |
|------|------|------|----------|
| `~>` | Pessimistic (推荐) | `~> 6.0` | `>= 6.0.0, < 7.0.0` |
| `>=` | Greater or equal | `>= 6.0` | `>= 6.0.0` |
| `>` | Greater than | `> 6.0` | `> 6.0.0` |
| `=` | Exact | `= 6.0.0` | `= 6.0.0` |
| `<=` | Less or equal | `<= 6.0` | `<= 6.0.0` |
| `<` | Less than | `< 7.0` | `< 7.0.0` |

### 版本冲突处理

**场景**: Workspace的Provider版本与Resource的Provider版本不一致

**处理策略**:
1. **Terraform自动选择**: Terraform会自动选择满足所有约束的最新版本
2. **冲突检测**: 如果无法满足所有约束，terraform init会失败
3. **用户修改**: 用户需要修改Workspace或Resource的版本约束

**示例**:
```
Workspace: aws ~> 6.0  (允许 6.0.0 - 6.99.99)
Resource:  aws ~> 5.0  (允许 5.0.0 - 5.99.99)
结果: 冲突！无交集

解决方案:
- 修改Workspace为 ~> 5.0
- 或修改Resource为 ~> 6.0
```

## 📋 API接口设计

### Provider配置API

```go
// 获取Provider配置（隐藏敏感信息）
GET /api/v1/workspaces/:id/provider-config
Response: {
  "provider": {
    "aws": [
      {
        "alias": "us-east",
        "region": "us-east-1",
        "access_key": "***HIDDEN***",
        "secret_key": "***HIDDEN***"
      }
    ]
  },
  "terraform": [...]
}

// 更新Provider配置
PATCH /api/v1/workspaces/:id
Request: {
  "provider_config": {
    "provider": {...},
    "terraform": [...]
  }
}

// 验证Provider配置
POST /api/v1/workspaces/:id/provider-config/validate
Request: {
  "provider_config": {...}
}
Response: {
  "valid": true,
  "errors": []
}

// 测试Provider连接
POST /api/v1/workspaces/:id/provider-config/test
Request: {
  "provider_type": "aws",
  "alias": "us-east"
}
Response: {
  "success": true,
  "message": "Successfully connected to AWS us-east-1"
}
```

## 🔄 高级参数处理

### 参数值类型解析

```typescript
// 解析高级参数值
const parseParamValue = (value: string): any => {
  // 尝试解析为JSON
  try {
    return JSON.parse(value);
  } catch {
    // 不是JSON，尝试其他类型
    
    // 布尔值
    if (value === 'true') return true;
    if (value === 'false') return false;
    
    // 数字
    if (/^\d+$/.test(value)) return parseInt(value, 10);
    if (/^\d+\.\d+$/.test(value)) return parseFloat(value);
    
    // 字符串
    return value;
  }
};

// 处理参数值变更
const handleParamValueChange = (index: number, value: string) => {
  const params = { ...formData.advancedParams };
  const keys = Object.keys(params);
  const key = keys[index];
  
  // 解析值类型
  params[key] = parseParamValue(value);
  
  setFormData({ ...formData, advancedParams: params });
  setHasChanges(true);
};
```

### 常用高级参数示例

#### AWS Provider高级参数
```json
{
  "max_retries": 5,
  "skip_credentials_validation": false,
  "skip_metadata_api_check": false,
  "skip_region_validation": false,
  "skip_requesting_account_id": false,
  "default_tags": [
    {
      "tags": {
        "Environment": "Production",
        "ManagedBy": "Terraform",
        "Team": "Platform"
      }
    }
  ],
  "ignore_tags": [
    {
      "keys": ["IgnoreMe"],
      "key_prefixes": ["temp-"]
    }
  ],
  "endpoints": [
    {
      "s3": "https://s3.custom-endpoint.com",
      "ec2": "https://ec2.custom-endpoint.com"
    }
  ]
}
```

## 🎨 UI样式规范

### Provider卡片样式

```css
.providerCard {
  background: var(--color-white);
  border: 1px solid var(--color-gray-200);
  border-radius: var(--radius-lg);
  padding: 20px;
  margin-bottom: 16px;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.05);
  transition: all 0.2s;
}

.providerCard:hover {
  box-shadow: 0 4px 6px rgba(0, 0, 0, 0.07);
}

.cardHeader {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 16px;
  padding-bottom: 12px;
  border-bottom: 1px solid var(--color-gray-100);
}

.cardTitle {
  display: flex;
  align-items: center;
  gap: 12px;
}

.providerIcon {
  font-size: 24px;
}

.providerName {
  font-size: 18px;
  font-weight: 600;
  color: var(--color-gray-900);
  text-transform: uppercase;
}

.aliasBadge {
  background: var(--color-blue-100);
  color: var(--color-blue-700);
  padding: 4px 12px;
  border-radius: var(--radius-sm);
  font-size: 12px;
  font-weight: 600;
}

.cardActions {
  display: flex;
  gap: 8px;
}

.editButton,
.deleteButton {
  padding: 6px 12px;
  border-radius: var(--radius-md);
  font-size: 14px;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.2s;
}

.editButton {
  background: var(--color-white);
  border: 1px solid var(--color-gray-300);
  color: var(--color-gray-700);
}

.editButton:hover {
  background: var(--color-gray-50);
}

.deleteButton {
  background: var(--color-white);
  border: 1px solid var(--color-red-300);
  color: var(--color-red-600);
}

.deleteButton:hover {
  background: var(--color-red-50);
}

.configRow {
  display: flex;
  gap: 12px;
  margin-bottom: 8px;
  font-size: 14px;
}

.configLabel {
  color: var(--color-gray-600);
  font-weight: 500;
  min-width: 120px;
}

.configValue {
  color: var(--color-gray-900);
}
```

### 高级参数输入样式

```css
.advancedParams {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.paramRow {
  display: grid;
  grid-template-columns: 1fr 2fr auto;
  gap: 12px;
  align-items: start;
}

.paramKey,
.paramValue {
  padding: 10px 12px;
  border: 1px solid var(--color-gray-300);
  border-radius: var(--radius-md);
  font-size: 14px;
  transition: all 0.2s;
}

.paramKey:focus,
.paramValue:focus {
  outline: none;
  border-color: var(--color-blue-600);
  box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.1);
}

.paramValue {
  font-family: var(--font-mono);
  resize: vertical;
}

.removeParamButton {
  background: var(--color-red-50);
  border: 1px solid var(--color-red-200);
  color: var(--color-red-600);
  width: 32px;
  height: 32px;
  border-radius: var(--radius-md);
  font-size: 20px;
  cursor: pointer;
  transition: all 0.2s;
}

.removeParamButton:hover {
  background: var(--color-red-100);
}

.addParamButton {
  background: var(--color-white);
  border: 1px solid var(--color-gray-300);
  color: var(--color-gray-700);
  padding: 8px 16px;
  border-radius: var(--radius-md);
  font-size: 14px;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.2s;
}

.addParamButton:hover {
  background: var(--color-gray-50);
}
```

## 🧪 测试场景

### 功能测试

1. **添加Provider配置**
   - 添加IAM Role方式的AWS provider
   - 添加AKSK方式的AWS provider
   - 添加Assume Role方式的AWS provider
   - 验证alias唯一性

2. **编辑Provider配置**
   - 修改region
   - 切换认证方式
   - 添加/删除高级参数
   - 修改版本约束

3. **删除Provider配置**
   - 删除带alias的provider
   - 删除默认provider
   - 验证删除后的配置有效性

4. **高级参数**
   - 添加string类型参数
   - 添加number类型参数
   - 添加boolean类型参数
   - 添加JSON对象参数

5. **版本约束**
   - 测试各种约束符号
   - 验证版本冲突检测
   - 测试terraform init是否使用正确版本

### 集成测试

1. **Fetching阶段测试**
   - 验证provider.tf.json生成正确
   - 验证敏感信息不泄露
   - 验证多provider配置正确

2. **执行测试**
   - IAM Role方式执行成功
   - AKSK方式执行成功
   - Assume Role方式执行成功
   - 多provider配置执行成功

## 📝 实施清单

### 后端任务
- [ ] 实现Provider配置验证服务
- [ ] 实现敏感信息过滤
- [ ] 实现Provider连接测试
- [ ] 更新Fetching阶段生成provider.tf.json
- [ ] 添加Provider配置API端点

### 前端任务
- [ ] 创建Provider Settings子页面
- [ ] 实现ProviderCard组件
- [ ] 实现ProviderForm组件
- [ ] 实现高级参数输入组件
- [ ] 实现版本约束选择器
- [ ] 集成到Settings页面导航
- [ ] 实现保存和验证逻辑

### 测试任务
- [ ] 单元测试（配置验证）
- [ ] 集成测试（provider.tf.json生成）
- [ ] E2E测试（完整执行流程）
- [ ] 安全测试（敏感信息保护）

## 🔗 相关文档

- [13-workspace-settings-design.md](./13-workspace-settings-design.md) - Settings页面设计
- [15-terraform-execution-detail.md](./15-terraform-execution-detail.md) - Terraform执行流程
- [08-database-design.md](./08-database-design.md) - 数据库设计

## 📊 数据流图

```
用户配置Provider
    ↓
保存到workspaces.provider_config (JSONB)
    ↓
Fetching阶段读取配置
    ↓
生成provider.tf.json文件
    ↓
Terraform Init下载Provider插件
    ↓
Plan/Apply使用Provider认证
```

##  重要注意事项

1. **Alias唯一性**: 同一Provider类型的alias必须唯一
2. **默认Provider**: 每个Provider类型只能有一个默认配置（无alias）
3. **敏感信息**: access_key和secret_key在API响应中隐藏
4. **版本冲突**: 用户需要手动解决版本约束冲突
5. **高级参数**: 支持任意key-value对，value可以是复杂类型

---

**实施优先级**: 高（Terraform执行的必需功能）
**预计工作量**: 2-3天
**依赖**: 无
**风险**: 低
