// 兼容性表单渲染器
// 自动检测 Schema 版本并选择对应的渲染器

import React, { useMemo } from 'react';
import { Alert, Button, Space, Tag } from 'antd';
import { ArrowUpOutlined, InfoCircleOutlined } from '@ant-design/icons';
import { detectSchemaVersion } from '../../services/schemaVersionDetector';
import { FormRenderer as OpenAPIFormRenderer } from '../OpenAPIFormRenderer';
import type { OpenAPIFormSchema, AIAssistantConfig } from '../OpenAPIFormRenderer/types';

interface ManifestContext {
  currentNodeId: string;
  connectedNodeIds?: string[];  // 已连线的节点 ID 列表（只有这些节点可以被引用）
  nodes: Array<{
    id: string;
    instance_name: string;
    module_id?: number;
    module_source?: string;
    outputs?: Array<{ name: string; type?: string; description?: string }>;
  }>;
  onAddEdge?: (sourceNodeId: string, targetNodeId: string, sourceOutput: string, targetInput: string) => void;
}

interface ModuleFormRendererProps {
  schema: unknown;
  initialValues?: Record<string, unknown>;
  onChange?: (values: Record<string, unknown>) => void;
  onSubmit?: (values: Record<string, unknown>) => void;
  disabled?: boolean;
  readOnly?: boolean;
  showVersionBadge?: boolean;
  onMigrate?: () => void;
  workspace?: { id: string; name: string };
  organization?: { id: string; name: string };
  manifest?: ManifestContext;  // Manifest 编辑器上下文
  activeGroupId?: string;  // 当前活跃的分组 ID（用于 tabs 布局的 URL 参数持久化）
  onGroupChange?: (groupId: string) => void;  // 分组切换回调
  aiAssistant?: AIAssistantConfig;  // AI 助手配置
}

const ModuleFormRenderer: React.FC<ModuleFormRendererProps> = ({
  schema,
  initialValues = {},
  onChange,
  onSubmit,
  disabled = false,
  readOnly = false,
  showVersionBadge = true,
  onMigrate,
  workspace,
  organization,
  manifest,
  activeGroupId,
  onGroupChange,
  aiAssistant,
}) => {
  // 检测 Schema 版本
  const schemaVersion = useMemo(() => detectSchemaVersion(schema), [schema]);
  const isV2 = schemaVersion === 'v2';

  // 渲染版本标识
  const renderVersionBadge = () => {
    if (!showVersionBadge) return null;

    return (
      <div style={{ marginBottom: 16 }}>
        <Space>
          <Tag color={isV2 ? 'green' : 'orange'}>
            Schema {schemaVersion.toUpperCase()}
          </Tag>
          {!isV2 && onMigrate && (
            <Button
              type="link"
              size="small"
              icon={<ArrowUpOutlined />}
              onClick={onMigrate}
            >
              升级到 V2
            </Button>
          )}
        </Space>
      </div>
    );
  };

  // 渲染 V1 Schema 提示
  const renderV1Warning = () => {
    if (isV2) return null;

    return (
      <Alert
        type="info"
        showIcon
        icon={<InfoCircleOutlined />}
        message="使用传统 Schema 格式"
        description={
          <span>
            当前模块使用 V1 Schema 格式。建议升级到 V2 (OpenAPI) 格式以获得更好的表单渲染体验和更多功能支持。
            {onMigrate && (
              <Button type="link" size="small" onClick={onMigrate}>
                立即升级
              </Button>
            )}
          </span>
        }
        style={{ marginBottom: 16 }}
      />
    );
  };

  // 渲染 V2 表单
  if (isV2) {
    return (
      <div>
        {renderVersionBadge()}
        <OpenAPIFormRenderer
          schema={schema as OpenAPIFormSchema}
          initialValues={initialValues}
          onChange={onChange}
          onSubmit={onSubmit}
          disabled={disabled}
          readOnly={readOnly}
          workspace={workspace}
          organization={organization}
          manifest={manifest}
          activeGroupId={activeGroupId}
          onGroupChange={onGroupChange}
          aiAssistant={aiAssistant}
        />
      </div>
    );
  }

  // 渲染 V1 表单（使用旧的 DynamicForm 或简单展示）
  return (
    <div>
      {renderVersionBadge()}
      {renderV1Warning()}
      <V1FormRenderer
        schema={schema}
        initialValues={initialValues}
        onChange={onChange}
        onSubmit={onSubmit}
        disabled={disabled}
        readOnly={readOnly}
      />
    </div>
  );
};

// V1 表单渲染器（简化版，保持向后兼容）
interface V1FormRendererProps {
  schema: unknown;
  initialValues?: Record<string, unknown>;
  onChange?: (values: Record<string, unknown>) => void;
  onSubmit?: (values: Record<string, unknown>) => void;
  disabled?: boolean;
  readOnly?: boolean;
}

const V1FormRenderer: React.FC<V1FormRendererProps> = ({
  schema,
}) => {
  // V1 Schema 的简单渲染
  // 实际项目中应该使用现有的 DynamicForm 组件
  const schemaObj = schema as Record<string, unknown>;
  const fields = Object.entries(schemaObj).filter(([key]) => 
    !['openapi', 'info', 'components', 'x-iac-platform'].includes(key)
  );

  if (fields.length === 0) {
    return (
      <Alert
        type="warning"
        message="无法解析 Schema"
        description="Schema 格式无法识别，请检查数据格式或升级到 V2 版本。"
      />
    );
  }

  return (
    <div style={{ padding: 16, background: '#fafafa', borderRadius: 8 }}>
      <Alert
        type="info"
        message="V1 Schema 预览"
        description={`检测到 ${fields.length} 个字段。建议升级到 V2 格式以获得完整的表单渲染功能。`}
        style={{ marginBottom: 16 }}
      />
      <pre style={{ 
        background: '#f5f5f5', 
        padding: 16, 
        borderRadius: 4,
        maxHeight: 400,
        overflow: 'auto',
        fontSize: 12,
      }}>
        {JSON.stringify(schema, null, 2)}
      </pre>
    </div>
  );
};

export default ModuleFormRenderer;
export { ModuleFormRenderer };
