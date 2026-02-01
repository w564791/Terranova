# CMDB外部数据源功能设计方案

## 1. 功能概述

### 1.1 目标
为OpenAPI Schema的字段配置CMDB数据源功能，让用户在填写表单时可以从CMDB中搜索并选择已有的云资源，同时**保留用户自由输入的能力**。

### 1.2 核心原则
- **CMDB是辅助功能，不是限制功能** - 提供搜索便利，但不限制用户输入
- **用户可以从CMDB搜索选择** - 这是便利功能
- **用户也可以直接手动输入任意值** - 这是必须保留的能力
- **不做任何值的校验限制** - 用户输入什么就是什么

## 2. 数据模型设计

### 2.1 扩展 ExternalDataSource 类型

```typescript
// frontend/src/components/OpenAPIFormRenderer/types.ts

export interface ExternalDataSource {
  id: string;
  type: 'api' | 'static' | 'terraform' | 'cmdb';  // 新增 cmdb 类型
  
  // API 类型配置
  api?: string;
  method?: string;
  params?: Record<string, string>;
  
  // 静态数据配置
  data?: Array<{ value: string; label: string }>;
  
  // CMDB 类型配置 (新增)
  cmdb?: CMDBSourceConfig;
  
  // 通用配置
  cache?: {
    ttl: number;
    key?: string;
  };
  transform?: {
    type: 'jmespath' | 'jsonpath';
    expression: string;
  };
  dependsOn?: string[];
}

// CMDB 数据源配置
export interface CMDBSourceConfig {
  enabled: boolean;              // 功能开关
  resourceType: string;          // 资源类型，如 "aws_security_group"
  valueField: CMDBValueField;    // 值字段配置
  labelField?: string;           // 显示标签字段，默认 "name"
  searchFields?: string[];       // 可搜索的字段列表
  filters?: CMDBFilters;         // 额外过滤条件
}

// 值字段类型
export type CMDBValueField = 'id' | 'arn' | 'name' | string;

// CMDB 过滤条件
export interface CMDBFilters {
  workspace_id?: string;         // 限制特定 workspace
  tags?: Record<string, string>; // 按标签过滤
}
```

### 2.2 扩展 FieldUIConfig

```typescript
// frontend/src/services/schemaV2.ts

export interface FieldUIConfig {
  label?: string;
  group?: string;
  widget?: string;
  help?: string;
  order?: number;
  placeholder?: string;
  
  // 外部数据源配置
  source?: string;              // 数据源 ID 引用
  externalSource?: string;      // 兼容旧配置
  
  // CMDB 快捷配置 (新增)
  cmdbSource?: {
    enabled: boolean;
    resourceType: string;
    valueField: CMDBValueField;
    labelField?: string;
  };
  
  // 搜索和自定义输入
  searchable?: boolean;         // 支持搜索
  allowCustom?: boolean;        // 允许自定义输入 (CMDB场景下默认为 true)
  
  // 其他配置...
  readonly?: boolean;
  hidden?: boolean;
  hiddenByDefault?: boolean;
  refreshButton?: boolean;
  editWarning?: string;
}
```

### 2.3 Schema 配置示例

```json
{
  "openapi": "3.0.0",
  "info": {
    "title": "EC2 Module",
    "version": "1.0.0"
  },
  "components": {
    "schemas": {
      "ModuleInput": {
        "type": "object",
        "properties": {
          "vpc_security_group_ids": {
            "type": "array",
            "items": { "type": "string" },
            "description": "Security Group IDs for the instance"
          },
          "iam_instance_profile": {
            "type": "string",
            "description": "IAM Instance Profile ARN"
          },
          "subnet_id": {
            "type": "string",
            "description": "Subnet ID for the instance"
          }
        }
      }
    }
  },
  "x-iac-platform": {
    "ui": {
      "fields": {
        "vpc_security_group_ids": {
          "label": "安全组",
          "widget": "multi-select",
          "group": "network",
          "searchable": true,
          "allowCustom": true,
          "cmdbSource": {
            "enabled": true,
            "resourceType": "aws_security_group",
            "valueField": "id",
            "labelField": "name"
          }
        },
        "iam_instance_profile": {
          "label": "IAM实例配置文件",
          "widget": "select",
          "group": "security",
          "searchable": true,
          "allowCustom": true,
          "cmdbSource": {
            "enabled": true,
            "resourceType": "aws_iam_instance_profile",
            "valueField": "arn",
            "labelField": "name"
          }
        },
        "subnet_id": {
          "label": "子网",
          "widget": "select",
          "group": "network",
          "searchable": true,
          "allowCustom": true,
          "cmdbSource": {
            "enabled": true,
            "resourceType": "aws_subnet",
            "valueField": "id",
            "labelField": "name"
          }
        }
      },
      "groups": [
        { "id": "network", "label": "网络配置", "order": 1 },
        { "id": "security", "label": "安全配置", "order": 2 }
      ]
    },
    "external": {
      "sources": [
        {
          "id": "security_groups",
          "type": "cmdb",
          "cmdb": {
            "enabled": true,
            "resourceType": "aws_security_group",
            "valueField": "id",
            "labelField": "name",
            "searchFields": ["id", "name", "description"]
          }
        },
        {
          "id": "iam_profiles",
          "type": "cmdb",
          "cmdb": {
            "enabled": true,
            "resourceType": "aws_iam_instance_profile",
            "valueField": "arn",
            "labelField": "name"
          }
        },
        {
          "id": "subnets",
          "type": "cmdb",
          "cmdb": {
            "enabled": true,
            "resourceType": "aws_subnet",
            "valueField": "id",
            "labelField": "name"
          }
        }
      ]
    }
  }
}
```

## 3. 预定义的资源类型映射

### 3.1 默认值字段映射表

| 资源类型 | 默认 valueField | 默认 labelField | 说明 |
|---------|----------------|-----------------|------|
| `aws_security_group` | `id` | `name` | 安全组使用 sg-xxx 格式的 ID |
| `aws_iam_policy` | `arn` | `name` | IAM 策略使用 ARN |
| `aws_iam_role` | `arn` | `name` | IAM 角色使用 ARN |
| `aws_iam_instance_profile` | `arn` | `name` | 实例配置文件使用 ARN |
| `aws_subnet` | `id` | `name` | 子网使用 subnet-xxx 格式的 ID |
| `aws_vpc` | `id` | `name` | VPC 使用 vpc-xxx 格式的 ID |
| `aws_s3_bucket` | `id` | `name` | S3 桶使用桶名作为 ID |
| `aws_kms_key` | `arn` | `name` | KMS 密钥使用 ARN |
| `aws_lb` | `arn` | `name` | 负载均衡器使用 ARN |
| `aws_lb_target_group` | `arn` | `name` | 目标组使用 ARN |
| `aws_efs_file_system` | `id` | `name` | EFS 使用 fs-xxx 格式的 ID |
| `aws_ebs_volume` | `id` | `name` | EBS 卷使用 vol-xxx 格式的 ID |
| `aws_ami` | `id` | `name` | AMI 使用 ami-xxx 格式的 ID |
| `aws_key_pair` | `name` | `name` | 密钥对使用名称 |
| `aws_acm_certificate` | `arn` | `name` | ACM 证书使用 ARN |
| `aws_route53_zone` | `id` | `name` | Route53 托管区使用 Zone ID |
| `aws_cloudwatch_log_group` | `name` | `name` | CloudWatch 日志组使用名称 |
| `aws_sns_topic` | `arn` | `name` | SNS 主题使用 ARN |
| `aws_sqs_queue` | `url` | `name` | SQS 队列使用 URL |
| `aws_dynamodb_table` | `name` | `name` | DynamoDB 表使用名称 |
| `aws_rds_cluster` | `id` | `name` | RDS 集群使用集群标识符 |
| `aws_db_instance` | `id` | `name` | RDS 实例使用实例标识符 |
| `aws_elasticache_cluster` | `id` | `name` | ElastiCache 集群使用集群 ID |
| `aws_eks_cluster` | `name` | `name` | EKS 集群使用名称 |

### 3.2 值字段来源映射

CMDB `resource_index` 表中的字段与 valueField 的对应关系：

| valueField | 对应 resource_index 字段 | 说明 |
|------------|-------------------------|------|
| `id` | `cloud_resource_id` | 云资源 ID (如 sg-xxx, subnet-xxx) |
| `arn` | `cloud_resource_arn` | AWS ARN |
| `name` | `cloud_resource_name` | 资源名称 |
| 自定义 | `attributes->>'字段名'` | 从 attributes JSON 中提取 |

## 4. 后端 API 设计

### 4.1 新增 CMDB 搜索 API

```
GET /api/v1/cmdb/search/options
```

**请求参数：**
```typescript
interface CMDBOptionsRequest {
  resource_type: string;      // 必填：资源类型
  value_field: string;        // 必填：值字段 (id/arn/name/自定义)
  label_field?: string;       // 可选：标签字段，默认 name
  query?: string;             // 可选：搜索关键词
  workspace_id?: string;      // 可选：限制 workspace
  limit?: number;             // 可选：返回数量限制，默认 50
}
```

**响应格式：**
```typescript
interface CMDBOptionsResponse {
  options: Array<{
    value: string;            // 选项值 (根据 value_field 提取)
    label: string;            // 显示标签 (根据 label_field 提取)
    description?: string;     // 资源描述
    workspace_id?: string;    // 所属 workspace
    workspace_name?: string;  // workspace 名称
    extra?: {                 // 额外信息
      cloud_id?: string;
      cloud_arn?: string;
      cloud_name?: string;
    };
  }>;
  total: number;              // 总数
  has_more: boolean;          // 是否还有更多
}
```

### 4.2 后端实现

```go
// backend/internal/handlers/cmdb_handler.go

// GetCMDBOptions 获取 CMDB 资源选项列表
func (h *CMDBHandler) GetCMDBOptions(c *gin.Context) {
    resourceType := c.Query("resource_type")
    valueField := c.DefaultQuery("value_field", "id")
    labelField := c.DefaultQuery("label_field", "name")
    query := c.Query("query")
    workspaceID := c.Query("workspace_id")
    limit := c.DefaultQuery("limit", "50")
    
    if resourceType == "" {
        c.JSON(400, gin.H{"error": "resource_type is required"})
        return
    }
    
    options, total, err := h.cmdbService.GetResourceOptions(
        resourceType, valueField, labelField, query, workspaceID, limit,
    )
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    
    c.JSON(200, gin.H{
        "options":  options,
        "total":    total,
        "has_more": total > len(options),
    })
}
```

```go
// backend/services/cmdb_service.go

// GetResourceOptions 获取资源选项列表
func (s *CMDBService) GetResourceOptions(
    resourceType, valueField, labelField, query, workspaceID string, limit int,
) ([]ResourceOption, int64, error) {
    
    db := s.db.Model(&models.ResourceIndex{}).
        Where("resource_type = ?", resourceType)
    
    if workspaceID != "" {
        db = db.Where("workspace_id = ?", workspaceID)
    }
    
    if query != "" {
        searchPattern := "%" + query + "%"
        db = db.Where(
            "cloud_resource_id ILIKE ? OR cloud_resource_name ILIKE ? OR description ILIKE ?",
            searchPattern, searchPattern, searchPattern,
        )
    }
    
    var total int64
    db.Count(&total)
    
    var resources []models.ResourceIndex
    if err := db.Limit(limit).Find(&resources).Error; err != nil {
        return nil, 0, err
    }
    
    options := make([]ResourceOption, 0, len(resources))
    for _, r := range resources {
        option := ResourceOption{
            Value:         s.extractValue(r, valueField),
            Label:         s.extractLabel(r, labelField),
            Description:   r.Description,
            WorkspaceID:   r.WorkspaceID,
            Extra: map[string]string{
                "cloud_id":   r.CloudResourceID,
                "cloud_arn":  r.CloudResourceARN,
                "cloud_name": r.CloudResourceName,
            },
        }
        options = append(options, option)
    }
    
    return options, total, nil
}

// extractValue 根据 valueField 提取值
func (s *CMDBService) extractValue(r models.ResourceIndex, valueField string) string {
    switch valueField {
    case "id":
        return r.CloudResourceID
    case "arn":
        return r.CloudResourceARN
    case "name":
        return r.CloudResourceName
    default:
        // 尝试从 attributes 中提取
        if r.Attributes != nil {
            var attrs map[string]interface{}
            if err := json.Unmarshal(r.Attributes, &attrs); err == nil {
                if val, ok := attrs[valueField]; ok {
                    return fmt.Sprintf("%v", val)
                }
            }
        }
        return r.CloudResourceID
    }
}

// extractLabel 根据 labelField 提取标签
func (s *CMDBService) extractLabel(r models.ResourceIndex, labelField string) string {
    switch labelField {
    case "name":
        if r.CloudResourceName != "" {
            return r.CloudResourceName
        }
        return r.CloudResourceID
    case "id":
        return r.CloudResourceID
    case "arn":
        return r.CloudResourceARN
    default:
        if r.Attributes != nil {
            var attrs map[string]interface{}
            if err := json.Unmarshal(r.Attributes, &attrs); err == nil {
                if val, ok := attrs[labelField]; ok {
                    return fmt.Sprintf("%v", val)
                }
            }
        }
        return r.CloudResourceName
    }
}
```

## 5. 前端实现设计

### 5.1 CMDB 数据源服务

```typescript
// frontend/src/services/cmdb.ts

// 新增：获取 CMDB 资源选项
export interface CMDBOption {
  value: string;
  label: string;
  description?: string;
  workspace_id?: string;
  workspace_name?: string;
  extra?: {
    cloud_id?: string;
    cloud_arn?: string;
    cloud_name?: string;
  };
}

export interface CMDBOptionsResponse {
  options: CMDBOption[];
  total: number;
  has_more: boolean;
}

export const cmdbService = {
  // ... 现有方法 ...
  
  // 获取资源选项列表
  getResourceOptions: async (params: {
    resource_type: string;
    value_field: string;
    label_field?: string;
    query?: string;
    workspace_id?: string;
    limit?: number;
  }): Promise<CMDBOptionsResponse> => {
    const searchParams = new URLSearchParams();
    searchParams.append('resource_type', params.resource_type);
    searchParams.append('value_field', params.value_field);
    if (params.label_field) searchParams.append('label_field', params.label_field);
    if (params.query) searchParams.append('query', params.query);
    if (params.workspace_id) searchParams.append('workspace_id', params.workspace_id);
    if (params.limit) searchParams.append('limit', params.limit.toString());
    
    return api.get(`/cmdb/search/options?${searchParams.toString()}`);
  },
};
```

### 5.2 Schema 编辑器 - 外部数据源配置组件

```typescript
// frontend/src/components/OpenAPISchemaEditor/ExternalSourceConfig.tsx

import React, { useState, useEffect } from 'react';
import { cmdbService } from '../../services/cmdb';
import styles from './OpenAPISchemaEditor.module.css';

interface ExternalSourceConfigProps {
  uiConfig: any;
  onChange: (uiConfig: any) => void;
}

// 预定义的资源类型和默认值字段映射
const RESOURCE_TYPE_DEFAULTS: Record<string, { valueField: string; labelField: string }> = {
  'aws_security_group': { valueField: 'id', labelField: 'name' },
  'aws_iam_policy': { valueField: 'arn', labelField: 'name' },
  'aws_iam_role': { valueField: 'arn', labelField: 'name' },
  'aws_iam_instance_profile': { valueField: 'arn', labelField: 'name' },
  'aws_subnet': { valueField: 'id', labelField: 'name' },
  'aws_vpc': { valueField: 'id', labelField: 'name' },
  'aws_s3_bucket': { valueField: 'id', labelField: 'name' },
  'aws_kms_key': { valueField: 'arn', labelField: 'name' },
  'aws_lb': { valueField: 'arn', labelField: 'name' },
  'aws_lb_target_group': { valueField: 'arn', labelField: 'name' },
  'aws_ami': { valueField: 'id', labelField: 'name' },
  'aws_key_pair': { valueField: 'name', labelField: 'name' },
  'aws_acm_certificate': { valueField: 'arn', labelField: 'name' },
  'aws_eks_cluster': { valueField: 'name', labelField: 'name' },
  'aws_rds_cluster': { valueField: 'id', labelField: 'name' },
  'aws_db_instance': { valueField: 'id', labelField: 'name' },
};

const VALUE_FIELD_OPTIONS = [
  { value: 'id', label: 'Resource ID (如 sg-xxx, subnet-xxx)' },
  { value: 'arn', label: 'ARN (如 arn:aws:iam::...)' },
  { value: 'name', label: 'Resource Name' },
  { value: 'custom', label: '自定义字段' },
];

export const ExternalSourceConfig: React.FC<ExternalSourceConfigProps> = ({
  uiConfig,
  onChange,
}) => {
  const [sourceType, setSourceType] = useState<'none' | 'static' | 'api' | 'cmdb'>('none');
  const [resourceTypes, setResourceTypes] = useState<string[]>([]);
  const [customValueField, setCustomValueField] = useState('');
  
  // 初始化状态
  useEffect(() => {
    if (uiConfig.cmdbSource?.enabled) {
      setSourceType('cmdb');
    } else if (uiConfig.source) {
      setSourceType('api');
    } else {
      setSourceType('none');
    }
  }, [uiConfig]);
  
  // 加载可用的资源类型
  useEffect(() => {
    cmdbService.getResourceTypes().then(res => {
      setResourceTypes(res.resource_types.map(r => r.resource_type));
    });
  }, []);
  
  const handleSourceTypeChange = (type: 'none' | 'static' | 'api' | 'cmdb') => {
    setSourceType(type);
    
    if (type === 'none') {
      const newConfig = { ...uiConfig };
      delete newConfig.source;
      delete newConfig.cmdbSource;
      onChange(newConfig);
    } else if (type === 'cmdb') {
      onChange({
        ...uiConfig,
        source: undefined,
        cmdbSource: {
          enabled: true,
          resourceType: '',
          valueField: 'id',
          labelField: 'name',
        },
        allowCustom: true,  // CMDB 场景下默认允许自定义输入
        searchable: true,
      });
    }
  };
  
  const handleCMDBConfigChange = (key: string, value: any) => {
    const newCmdbSource = { ...uiConfig.cmdbSource, [key]: value };
    
    // 当选择资源类型时，自动设置默认的值字段
    if (key === 'resourceType' && RESOURCE_TYPE_DEFAULTS[value]) {
      newCmdbSource.valueField = RESOURCE_TYPE_DEFAULTS[value].valueField;
      newCmdbSource.labelField = RESOURCE_TYPE_DEFAULTS[value].labelField;
    }
    
    onChange({ ...uiConfig, cmdbSource: newCmdbSource });
  };
  
  const handleValueFieldChange = (value: string) => {
    if (value === 'custom') {
      // 显示自定义输入框
      setCustomValueField('');
    } else {
      handleCMDBConfigChange('valueField', value);
    }
  };
  
  return (
    <div className={styles.externalSourceConfig}>
      <div className={styles.configSection}>
        <label>数据源类型</label>
        <select
          value={sourceType}
          onChange={(e) => handleSourceTypeChange(e.target.value as any)}
          className={styles.fieldSelect}
        >
          <option value="none">无 (用户自由输入)</option>
          <option value="static">静态选项 (static)</option>
          <option value="api">API 接口 (api)</option>
          <option value="cmdb">CMDB 资源 (cmdb)</option>
        </select>
      </div>
      
      {sourceType === 'cmdb' && (
        <div className={styles.cmdbConfig}>
          <div className={styles.configRow}>
            <div className={styles.configField}>
              <label>资源类型</label>
              <select
                value={uiConfig.cmdbSource?.resourceType || ''}
                onChange={(e) => handleCMDBConfigChange('resourceType', e.target.value)}
                className={styles.fieldSelect}
              >
                <option value="">请选择资源类型</option>
                {resourceTypes.map(type => (
                  <option key={type} value={type}>{type}</option>
                ))}
              </select>
            </div>
            
            <div className={styles.configField}>
              <label>值字段 (用户选择后提交的值)</label>
              <select
                value={uiConfig.cmdbSource?.valueField || 'id'}
                onChange={(e) => handleValueFieldChange(e.target.value)}
                className={styles.fieldSelect}
              >
                {VALUE_FIELD_OPTIONS.map(opt => (
                  <option key={opt.value} value={opt.value}>{opt.label}</option>
                ))}
              </select>
              {uiConfig.cmdbSource?.valueField === 'custom' && (
                <input
                  type="text"
                  value={customValueField}
                  onChange={(e) => {
                    setCustomValueField(e.target.value);
                    handleCMDBConfigChange('valueField', e.target.value);
                  }}
                  placeholder="输入自定义字段名"
                  className={styles.fieldInput}
                />
              )}
            </div>
          </div>
          
          <div className={styles.configRow}>
            <div className={styles.configField}>
              <label>显示字段 (下拉列表中显示的标签)</label>
              <select
                value={uiConfig.cmdbSource?.labelField || 'name'}
                onChange={(e) => handleCMDBConfigChange('labelField', e.target.value)}
                className={styles.fieldSelect}
              >
                <option value="name">Resource Name</option>
                <option value="id">Resource ID</option>
                <option value="arn">ARN</option>
              </select>
            </div>
          </div>
          
          <div className={styles.configHint}>
            <p>💡 <strong>重要说明：</strong></p>
            <ul>
              <li>CMDB 数据源是<strong>辅助功能</strong>，用户可以从 CMDB 搜索选择，也可以手动输入任意值</li>
              <li>值字段决定了用户选择 CMDB 资源后，实际提交的值是什么（如 sg-xxx 或 arn:aws:...）</li>
              <li>显示字段决定了下拉列表中显示的标签（通常是资源名称）</li>
            </ul>
          </div>
        </div>
      )}
      
      {sourceType === 'api' && (
        <div className={styles.apiConfig}>
          <div className={styles.configField}>
            <label>API 数据源 ID</label>
            <input
              type="text"
              value={uiConfig.source || ''}
              onChange={(e) => onChange({ ...uiConfig, source: e.target.value })}
              placeholder="例如：ami_list"
              className={styles.fieldInput}
            />
          </div>
        </div>
      )}
      
      {/* 通用选项 */}
      {sourceType !== 'none' && (
        <div className={styles.commonOptions}>
          <label className={styles.checkboxLabel}>
            <input
              type="checkbox"
              checked={uiConfig.allowCustom !== false}
              onChange={(e) => onChange({ ...uiConfig, allowCustom: e.target.checked })}
            />
            <span>允许用户自定义输入（不限制只能从列表选择）</span>
          </label>
          <label className={styles.checkboxLabel}>
            <input
              type="checkbox"
              checked={uiConfig.searchable || false}
              onChange={(e) => onChange({ ...uiConfig, searchable: e.target.checked })}
            />
            <span>支持搜索</span>
          </label>
        </div>
      )}
    </div>
  );
};
```

### 5.3 表单渲染器 - CMDB Select Widget

```typescript
// frontend/src/components/OpenAPIFormRenderer/widgets/CMDBSelectWidget.tsx

import React, { useState, useEffect, useCallback } from 'react';
import { Select, Input, Spin, Empty, Tag, Tooltip } from 'antd';
import { SearchOutlined, EditOutlined, DatabaseOutlined } from '@ant-design/icons';
import { cmdbService, CMDBOption } from '../../../services/cmdb';
import { debounce } from 'lodash';
import styles from './CMDBSelectWidget.module.css';

interface CMDBSelectWidgetProps {
  value?: string | string[];
  onChange?: (value: string | string[]) => void;
  multiple?: boolean;
  cmdbConfig: {
    resourceType: string;
    valueField: string;
    labelField?: string;
  };
  allowCustom?: boolean;
  searchable?: boolean;
  placeholder?: string;
  disabled?: boolean;
}

export const CMDBSelectWidget: React.FC<CMDBSelectWidgetProps> = ({
  value,
  onChange,
  multiple = false,
  cmdbConfig,
  allowCustom = true,  // 默认允许自定义输入
  searchable = true,
  placeholder,
  disabled,
}) => {
  const [options, setOptions] = useState<CMDBOption[]>([]);
  const [loading, setLoading] = useState(false);
  const [searchValue, setSearchValue] = useState('');
  const [inputMode, setInputMode] = useState<'select' | 'input'>('select');
  const [customInput, setCustomInput] = useState('');
  
  // 加载 CMDB 选项
  const loadOptions = useCallback(async (query?: string) => {
    if (!cmdbConfig.resourceType) return;
    
    setLoading(true);
    try {
      const response = await cmdbService.getResourceOptions({
        resource_type: cmdbConfig.resourceType,
        value_field: cmdbConfig.valueField,
        label_field: cmdbConfig.labelField,
        query,
        limit: 50,
      });
      setOptions(response.options);
    } catch (error) {
      console.error('Failed to load CMDB options:', error);
    } finally {
      setLoading(false);
    }
  }, [cmdbConfig]);
  
  // 初始加载
  useEffect(() => {
    loadOptions();
  }, [loadOptions]);
  
  // 防抖搜索
  const debouncedSearch = useCallback(
    debounce((query: string) => {
      loadOptions(query);
    }, 300),
    [loadOptions]
  );
  
  const handleSearch = (value: string) => {
    setSearchValue(value);
    if (searchable) {
      debouncedSearch(value);
    }
  };
  
  // 处理选择
  const handleSelect = (selectedValue: string) => {
    if (multiple) {
      const currentValues = Array.isArray(value) ? value : [];
      if (!currentValues.includes(selectedValue)) {
        onChange?.([...currentValues, selectedValue]);
      }
    } else {
      onChange?.(selectedValue);
    }
  };
  
  // 处理自定义输入
  const handleCustomInputConfirm = () => {
    if (!customInput.trim()) return;
    
    if (multiple) {
      const currentValues = Array.isArray(value) ? value : [];
      if (!currentValues.includes(customInput)) {
        onChange?.([...currentValues, customInput]);
      }
    } else {
      onChange?.(customInput);
    }
    setCustomInput('');
    setInputMode('select');
  };
  
  // 删除已选值
  const handleRemove = (removedValue: string) => {
    if (multiple) {
      const currentValues = Array.isArray(value) ? value : [];
      onChange?.(currentValues.filter(v => v !== removedValue));
    } else {
      onChange?.(undefined as any);
    }
  };
  
  // 渲染已选择的值
  const renderSelectedValues = () => {
    const values = multiple ? (Array.isArray(value) ? value : []) : (value ? [value] : []);
    
    return (
      <div className={styles.selectedValues}>
        {values.map(v => {
          const option = options.find(o => o.value === v);
          const isFromCMDB = !!option;
          
          return (
            <Tag
              key={v}
              closable={!disabled}
              onClose={() => handleRemove(v)}
              className={isFromCMDB ? styles.cmdbTag : styles.customTag}
            >
              {isFromCMDB && <DatabaseOutlined className={styles.tagIcon} />}
              {!isFromCMDB && <EditOutlined className={styles.tagIcon} />}
              <Tooltip title={isFromCMDB ? `来自 CMDB: ${option?.label}` : '手动输入'}>
                <span>{option?.label || v}</span>
              </Tooltip>
            </Tag>
          );
        })}
      </div>
    );
  };
  
  return (
    <div className={styles.cmdbSelectWidget}>
      {/* 已选择的值 */}
      {renderSelectedValues()}
      
      {/* 输入区域 */}
      <div className={styles.inputArea}>
        {inputMode === 'select' ? (
          <>
            <Select
              showSearch={searchable}
              loading={loading}
              placeholder={placeholder || `从 CMDB 搜索 ${cmdbConfig.resourceType}...`}
              onSearch={handleSearch}
              onSelect={handleSelect}
              filterOption={false}
              notFoundContent={loading ? <Spin size="small" /> : <Empty description="无匹配资源" />}
              disabled={disabled}
              value={undefined}  // 不绑定值，只用于选择
              className={styles.selectInput}
              dropdownRender={(menu) => (
                <div>
                  {menu}
                  {allowCustom && (
                    <div className={styles.dropdownFooter}>
                      <span className={styles.footerHint}>
                        💡 找不到？可以
                        <a onClick={() => setInputMode('input')}>手动输入</a>
                      </span>
                    </div>
                  )}
                </div>
              )}
            >
              {options.map(option => (
                <Select.Option key={option.value} value={option.value}>
                  <div className={styles.optionItem}>
                    <span className={styles.optionLabel}>{option.label}</span>
                    <span className={styles.optionValue}>{option.value}</span>
                    {option.workspace_name && (
                      <span className={styles.optionWorkspace}>
                        @ {option.workspace_name}
                      </span>
                    )}
                  </div>
                </Select.Option>
              ))}
            </Select>
            
            {allowCustom && (
              <Tooltip title="手动输入">
                <button
                  type="button"
                  onClick={() => setInputMode('input')}
                  className={styles.modeToggle}
                  disabled={disabled}
                >
                  <EditOutlined />
                </button>
              </Tooltip>
            )}
          </>
        ) : (
          <>
            <Input
              value={customInput}
              onChange={(e) => setCustomInput(e.target.value)}
              onPressEnter={handleCustomInputConfirm}
              placeholder="输入自定义值，按 Enter 确认"
              disabled={disabled}
              className={styles.customInputField}
            />
            <button
              type="button"
              onClick={handleCustomInputConfirm}
              className={styles.confirmButton}
              disabled={!customInput.trim() || disabled}
            >
              确认
            </button>
            <Tooltip title="从 CMDB 搜索">
              <button
                type="button"
                onClick={() => setInputMode('select')}
                className={styles.modeToggle}
                disabled={disabled}
              >
                <SearchOutlined />
              </button>
            </Tooltip>
          </>
        )}
      </div>
    </div>
  );
};
```

## 6. UI 交互设计

### 6.1 表单字段渲染效果

```
┌─────────────────────────────────────────────────────────────────┐
│ Security Group IDs                                               │
├─────────────────────────────────────────────────────────────────┤
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │ [🗄️] sg-abc123 (web-server-sg)  [x]                         │ │
│ │ [✏️] sg-custom-input            [x]                         │ │  ← 已选值（区分来源）
│ └─────────────────────────────────────────────────────────────┘ │
│                                                                 │
│ ┌───────────────────────────────────────────────┐ ┌───┐        │
│ │ 🔍 从 CMDB 搜索 aws_security_group...         │ │ ✏️ │        │  ← 搜索框 + 手动输入切换
│ └───────────────────────────────────────────────┘ └───┘        │
│                                                                 │
│ ┌─ 搜索结果 ────────────────────────────────────────────────┐  │
│ │ sg-111222    web-server-sg       @ workspace-prod         │  │
│ │ sg-333444    database-sg         @ workspace-prod         │  │
│ │ sg-555666    internal-sg         @ workspace-dev          │  │
│ ├───────────────────────────────────────────────────────────┤  │
│ │ 💡 找不到？可以 [手动输入]                                  │  │
│ └───────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
```

### 6.2 手动输入模式

```
┌─────────────────────────────────────────────────────────────────┐
│ Security Group IDs                                               │
├─────────────────────────────────────────────────────────────────┤
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │ [🗄️] sg-abc123 (web-server-sg)  [x]                         │ │
│ └─────────────────────────────────────────────────────────────┘ │
│                                                                 │
│ ┌───────────────────────────────────────┐ ┌────┐ ┌───┐        │
│ │ sg-my-custom-value                    │ │确认│ │ 🔍 │        │  ← 手动输入框
│ └───────────────────────────────────────┘ └────┘ └───┘        │
│                                                                 │
│ 💡 输入自定义值，按 Enter 或点击确认添加                         │
└─────────────────────────────────────────────────────────────────┘
```

### 6.3 值的来源标识

- **🗄️ 数据库图标** - 表示值来自 CMDB 选择
- **✏️ 编辑图标** - 表示值是手动输入的
- 两种来源的值在功能上完全等价，只是视觉上区分来源

## 7. 实现步骤

### 7.1 后端实现

1. **新增 API 端点**
   - `GET /api/v1/cmdb/search/options` - 获取资源选项列表
   - 修改 `backend/internal/handlers/cmdb_handler.go`
   - 修改 `backend/internal/router/router_cmdb.go`

2. **扩展 CMDB Service**
   - 添加 `GetResourceOptions` 方法
   - 添加 `extractValue` 和 `extractLabel` 辅助方法
   - 修改 `backend/services/cmdb_service.go`

### 7.2 前端实现

1. **扩展类型定义**
   - 修改 `frontend/src/components/OpenAPIFormRenderer/types.ts`
   - 修改 `frontend/src/services/schemaV2.ts`

2. **扩展 CMDB 服务**
   - 添加 `getResourceOptions` 方法
   - 修改 `frontend/src/services/cmdb.ts`

3. **Schema 编辑器改进**
   - 新增 `ExternalSourceConfig` 组件
   - 修改 `InlineFieldEditor` 中的 UI 配置部分
   - 修改 `frontend/src/components/OpenAPISchemaEditor/index.tsx`

4. **新增 CMDB Select Widget**
   - 创建 `frontend/src/components/OpenAPIFormRenderer/widgets/CMDBSelectWidget.tsx`
   - 创建 `frontend/src/components/OpenAPIFormRenderer/widgets/CMDBSelectWidget.module.css`

5. **集成到 FormRenderer**
   - 修改 `frontend/src/components/OpenAPIFormRenderer/FormRenderer.tsx`
   - 根据 `cmdbSource` 配置渲染 `CMDBSelectWidget`

## 8. 测试用例

### 8.1 功能测试

1. **CMDB 搜索功能**
   - 输入关键词能搜索到匹配的资源
   - 选择资源后，值正确填入表单
   - 值字段配置正确（id/arn/name）

2. **手动输入功能**
   - 可以切换到手动输入模式
   - 输入任意值后能正确添加
   - 手动输入的值不受 CMDB 数据限制

3. **混合使用**
   - 可以同时包含 CMDB 选择的值和手动输入的值
   - 删除值时正常工作
   - 提交表单时所有值都正确提交

### 8.2 边界测试

1. **CMDB 无数据时**
   - 显示空状态提示
   - 仍然可以手动输入

2. **网络错误时**
   - 显示错误提示
   - 仍然可以手动输入

3. **资源类型未配置时**
   - 不显示 CMDB 搜索功能
   - 只显示普通输入框

## 9. 后续优化

1. **缓存优化** - 缓存常用资源类型的选项列表
2. **权限控制** - 根据用户权限过滤可见的 workspace 资源
3. **跨 Workspace 搜索** - 支持配置是否允许跨 workspace 选择资源
4. **实时同步提示** - 当 CMDB 数据可能过期时提示用户
5. **批量导入** - 支持从 CMDB 批量选择多个资源
