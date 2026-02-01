import React from 'react';
import { Tabs, Empty, Tag } from 'antd';
import type { OpenAPISchema, ParseTFResponse } from '../../services/schemaV2';
import { extractFieldsFromSchema } from '../../services/schemaV2';
import styles from './ModuleSchemaV2.module.css';

// 模块输出定义
interface ModuleOutput {
  name: string;
  alias?: string;
  type: string;
  description?: string;
  sensitive?: boolean;
  valueExpression?: string;
}

interface SchemaPreviewProps {
  schema: OpenAPISchema | null;
  parseResult?: ParseTFResponse | null;
}

// 从 Schema 中提取 outputs 定义
const extractOutputsFromSchema = (schema: OpenAPISchema): ModuleOutput[] => {
  const outputs: ModuleOutput[] = [];

  // 方式1: 从 x-iac-platform.outputs.items 提取
  const iacPlatform = (schema as any)['x-iac-platform'];
  if (iacPlatform?.outputs?.items) {
    for (const item of iacPlatform.outputs.items) {
      outputs.push({
        name: item.name || '',
        alias: item.alias,
        type: item.type || 'string',
        description: item.description,
        sensitive: item.sensitive,
        valueExpression: item.valueExpression,
      });
    }
  }

  // 方式2: 从 components.schemas.ModuleOutput.properties 提取
  if (outputs.length === 0) {
    const schemas = schema.components?.schemas as any;
    const moduleOutput = schemas?.ModuleOutput;
    if (moduleOutput?.properties) {
      for (const [name, prop] of Object.entries(moduleOutput.properties)) {
        const propObj = prop as any;
        outputs.push({
          name,
          alias: propObj['x-alias'],
          type: propObj.type || 'string',
          description: propObj.description,
          sensitive: propObj['x-sensitive'],
          valueExpression: propObj['x-value-expression'],
        });
      }
    }
  }

  return outputs;
};

const SchemaPreview: React.FC<SchemaPreviewProps> = ({ schema, parseResult }) => {
  if (!schema) {
    return <Empty description="暂无 Schema 数据" />;
  }

  const fields = extractFieldsFromSchema(schema);
  const basicFields = fields.filter(f => f.uiConfig.group === 'basic');
  const advancedFields = fields.filter(f => f.uiConfig.group !== 'basic');
  const outputs = extractOutputsFromSchema(schema);

  const tabItems = [
    {
      key: 'overview',
      label: '概览',
      children: (
        <div>
          <div className={styles.previewStats}>
            <div className={styles.statItem}>
              <div className={styles.statValue}>{fields.length}</div>
              <div className={styles.statLabel}>总字段数</div>
            </div>
            <div className={styles.statItem}>
              <div className={styles.statValue}>{basicFields.length}</div>
              <div className={styles.statLabel}>基础配置</div>
            </div>
            <div className={styles.statItem}>
              <div className={styles.statValue}>{advancedFields.length}</div>
              <div className={styles.statLabel}>高级配置</div>
            </div>
            <div className={styles.statItem}>
              <div className={styles.statValue}>
                {schema.components?.schemas?.ModuleInput?.required?.length || 0}
              </div>
              <div className={styles.statLabel}>必填字段</div>
            </div>
          </div>

          <h4 style={{ marginTop: 24 }}>模块信息</h4>
          <table className={styles.annotationTable}>
            <tbody>
              <tr>
                <td><strong>名称</strong></td>
                <td>{schema.info?.title || '-'}</td>
              </tr>
              <tr>
                <td><strong>版本</strong></td>
                <td>{schema.info?.version || '-'}</td>
              </tr>
              <tr>
                <td><strong>Provider</strong></td>
                <td>{schema.info?.['x-provider'] || '-'}</td>
              </tr>
              <tr>
                <td><strong>描述</strong></td>
                <td>{schema.info?.description || '-'}</td>
              </tr>
            </tbody>
          </table>

          {parseResult?.warnings && parseResult.warnings.length > 0 && (
            <>
              <h4 style={{ marginTop: 24, color: '#faad14' }}>警告</h4>
              <ul>
                {parseResult.warnings.map((warning, index) => (
                  <li key={index} style={{ color: '#faad14' }}>{warning}</li>
                ))}
              </ul>
            </>
          )}
        </div>
      ),
    },
    {
      key: 'fields',
      label: '字段列表',
      children: (
        <div>
          <table className={styles.annotationTable}>
            <thead>
              <tr>
                <th>字段名</th>
                <th>类型</th>
                <th>分组</th>
                <th>必填</th>
                <th>默认值</th>
              </tr>
            </thead>
            <tbody>
              {fields.map(field => (
                <tr key={field.name}>
                  <td>
                    <code>{field.name}</code>
                    {field.uiConfig.label && (
                      <span style={{ color: '#8c8c8c', marginLeft: 8 }}>
                        ({field.uiConfig.label})
                      </span>
                    )}
                  </td>
                  <td>{field.property.type}</td>
                  <td>
                    <span className={`${styles.groupTag} ${field.uiConfig.group === 'basic' ? styles.basic : styles.advanced}`}>
                      {field.uiConfig.group === 'basic' ? '基础' : '高级'}
                    </span>
                  </td>
                  <td>
                    {schema.components?.schemas?.ModuleInput?.required?.includes(field.name) ? '是' : '否'}
                  </td>
                  <td>
                    {field.property.default !== undefined ? (
                      <code>{JSON.stringify(field.property.default)}</code>
                    ) : '-'}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ),
    },
    {
      key: 'outputs',
      label: `Outputs (${outputs.length})`,
      children: (
        <div>
          {outputs.length > 0 ? (
            <table className={styles.annotationTable}>
              <thead>
                <tr>
                  <th>输出名</th>
                  <th>类型</th>
                  <th>描述</th>
                  <th>属性</th>
                </tr>
              </thead>
              <tbody>
                {outputs.map(output => (
                  <tr key={output.name}>
                    <td>
                      <code>{output.name}</code>
                      {output.alias && (
                        <span style={{ color: '#8c8c8c', marginLeft: 8 }}>
                          ({output.alias})
                        </span>
                      )}
                    </td>
                    <td>
                      <Tag color="blue">{output.type}</Tag>
                    </td>
                    <td>{output.description || '-'}</td>
                    <td>
                      {output.sensitive && <Tag color="orange">Sensitive</Tag>}
                      {output.valueExpression && (
                        <code style={{ fontSize: '12px', color: '#666' }}>
                          {output.valueExpression}
                        </code>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          ) : (
            <Empty description="此模块暂无定义 Outputs" />
          )}
        </div>
      ),
    },
    {
      key: 'json',
      label: 'JSON Schema',
      children: (
        <pre className={styles.jsonPreview}>
          {JSON.stringify(schema, null, 2)}
        </pre>
      ),
    },
  ];

  return (
    <div className={styles.schemaPreview}>
      <Tabs items={tabItems} className={styles.previewTabs} />
    </div>
  );
};

export default SchemaPreview;
