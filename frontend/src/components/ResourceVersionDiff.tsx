import React, { useState, useEffect } from 'react';
import api from '../services/api';
import { useToast } from '../contexts/ToastContext';
import { useUIVersion } from '../hooks/useUIVersion';
import { jsonToHCL } from '../utils/hclFormatter';
// 注意：这里使用的是 Schema API V2（OpenAPI 格式），不是 UI 主题 V2
// Schema V2 是后端 API 数据格式版本，与 UI 主题 v2/v3 是两个不同的概念
import { schemaV2Service } from '../services/schemaV2';
import styles from './ResourceVersionDiff.module.css';

interface Version {
  id: number;
  version: number;
  change_summary: string;
  tf_code: any;
  is_latest?: boolean;
}

interface Props {
  isOpen: boolean;
  workspaceId: string;
  resourceId: string;
  fromVersion: number;
  toVersion: number;
  allVersions: Version[];
  onClose: () => void;
}

interface DiffField {
  field: string;
  type: 'added' | 'removed' | 'modified' | 'unchanged';
  oldValue?: any;
  newValue?: any;
}

const ResourceVersionDiff: React.FC<Props> = ({
  isOpen,
  workspaceId,
  resourceId,
  fromVersion: initialFromVersion,
  toVersion: initialToVersion,
  allVersions,
  onClose
}) => {
  const [fromVersion, setFromVersion] = useState(initialFromVersion);
  const [toVersion, setToVersion] = useState(initialToVersion);
  const [fromData, setFromData] = useState<any>(null);
  const [toData, setToData] = useState<any>(null);
  const [diffFields, setDiffFields] = useState<DiffField[]>([]);
  const [loading, setLoading] = useState(false);
  const [showUnchanged, setShowUnchanged] = useState(false);
  const [schema, setSchema] = useState<any>(null);
  const { showToast } = useToast();
  const { isV3 } = useUIVersion();

  useEffect(() => {
    if (isOpen) {
      loadSchema();
      loadVersionsAndCompare();
    }
  }, [isOpen, fromVersion, toVersion]);

  // schema 加载完成后，如果有版本数据，重新计算 diff（确保 typejsonstring 字段能被正确识别）
  useEffect(() => {
    if (schema && fromData && toData) {
      const diff = calculateDiff(
        extractModuleConfig(fromData.tf_code),
        extractModuleConfig(toData.tf_code)
      );
      setDiffFields(diff);
    }
  }, [schema]);

  // 加载资源的 module schema
  const loadSchema = async () => {
    try {
      // 1. 获取资源信息，拿到 module_source 和 module_version
      const resourceRes = await api.get(`/workspaces/${workspaceId}/resources/${resourceId}`);
      const resource = resourceRes.data || resourceRes;
      const tfCode = resource.tf_code;
      
      if (!tfCode || !tfCode.module) return;
      
      // 提取 module source
      const moduleKeys = Object.keys(tfCode.module);
      if (moduleKeys.length === 0) return;
      
      const moduleKey = moduleKeys[0];
      const moduleArray = tfCode.module[moduleKey];
      if (!Array.isArray(moduleArray) || moduleArray.length === 0) return;
      
      const moduleSource = moduleArray[0].source;
      if (!moduleSource) return;
      
      // 2. 通过 module_source 查找 module_id
      const modulesRes = await api.get('/modules', { params: { source: moduleSource } });
      const modules = modulesRes.data?.items || modulesRes.items || [];
      const module = modules.find((m: any) => m.source === moduleSource);
      
      if (!module) return;
      
      // 3. 加载 module 的 schema
      const schemaRes = await schemaV2Service.getSchemaV2(module.id);
      // getSchemaV2 返回 SchemaV2 对象，包含 openapi_schema 字段
      if (schemaRes && schemaRes.openapi_schema) {
        setSchema(schemaRes.openapi_schema);
      }
    } catch (error) {
      console.error('Failed to load schema:', error);
    }
  };

  const loadVersionsAndCompare = async () => {
    try {
      setLoading(true);
      
      // 加载两个版本的数据
      const [fromResponse, toResponse] = await Promise.all([
        api.get(`/workspaces/${workspaceId}/resources/${resourceId}/versions/${fromVersion}`),
        api.get(`/workspaces/${workspaceId}/resources/${resourceId}/versions/${toVersion}`)
      ]);
      
      const fromVersionData = (fromResponse as any).data?.version || (fromResponse as any).version || fromResponse;
      const toVersionData = (toResponse as any).data?.version || (toResponse as any).version || toResponse;
      
      setFromData(fromVersionData);
      setToData(toVersionData);
      
      // 计算差异
      const diff = calculateDiff(
        extractModuleConfig(fromVersionData.tf_code),
        extractModuleConfig(toVersionData.tf_code)
      );
      setDiffFields(diff);
    } catch (error: any) {
      showToast('加载版本数据失败', 'error');
      console.error('Failed to load versions:', error);
    } finally {
      setLoading(false);
    }
  };

  // 从tf_code中提取module配置
  const extractModuleConfig = (tfCode: any): any => {
    if (!tfCode || !tfCode.module) return {};

    const moduleKeys = Object.keys(tfCode.module);
    if (moduleKeys.length === 0) return {};

    const moduleKey = moduleKeys[0];
    const moduleArray = tfCode.module[moduleKey];

    if (Array.isArray(moduleArray) && moduleArray.length > 0) {
      const { source, ...config } = moduleArray[0];
      return config;
    }

    return {};
  };

  // 判断字段是否是 typejsonstring 类型（根据 schema）
  const isJsonStringField = (fieldName: string): boolean => {
    if (!schema) return false;
    
    const properties = schema.components?.schemas?.ModuleInput?.properties;
    if (!properties) return false;
    
    const fieldSchema = properties[fieldName];
    if (!fieldSchema) return false;
    
    return fieldSchema.format === 'json';
  };

  // 格式化 JSON 字符串为 jsonencode 格式（统一使用 pretty-print）
  const formatJsonStringAsHCL = (jsonStr: string): string => {
    try {
      const parsed = JSON.parse(jsonStr);
      // 先格式化为统一的 pretty-print JSON
      const prettyJson = JSON.stringify(parsed, null, 2);
      const lines: string[] = [];
      lines.push('jsonencode(');
      const inner = formatJsonEncodeValue(parsed, 1, (level: number) => '  '.repeat(level));
      lines.push(...inner);
      lines.push(')');
      return lines.join('\n');
    } catch {
      return jsonStr;
    }
  };

  // 格式化 jsonencode 内部的值
  const formatJsonEncodeValue = (value: any, level: number, pad: (level: number) => string): string[] => {
    const lines: string[] = [];
    const indent = pad(level);

    if (value === null || value === undefined) {
      lines.push(`${indent}null`);
      return lines;
    }

    if (typeof value === 'boolean' || typeof value === 'number') {
      lines.push(`${indent}${value}`);
      return lines;
    }

    if (typeof value === 'string') {
      lines.push(`${indent}"${value.replace(/"/g, '\\"')}"`);
      return lines;
    }

    if (Array.isArray(value)) {
      if (value.length === 0) {
        lines.push(`${indent}[]`);
        return lines;
      }
      lines.push(`${indent}[`);
      value.forEach((item, idx) => {
        const itemLines = formatJsonEncodeValue(item, level + 1, pad);
        const comma = idx < value.length - 1 ? ',' : '';
        if (itemLines.length === 1) {
          lines.push(`${itemLines[0]}${comma}`);
        } else {
          lines.push(...itemLines);
          if (comma) lines[lines.length - 1] += comma;
        }
      });
      lines.push(`${indent}]`);
      return lines;
    }

    if (typeof value === 'object') {
      const entries = Object.entries(value);
      if (entries.length === 0) {
        lines.push(`${indent}{}`);
        return lines;
      }
      lines.push(`${indent}{`);
      entries.forEach(([k, v], idx) => {
        const keyStr = /^[a-zA-Z_][a-zA-Z0-9_]*$/.test(k) ? k : `"${k}"`;
        const valueLines = formatJsonEncodeValue(v, level + 1, pad);
        if (valueLines.length === 1) {
          lines.push(`${pad(level + 1)}${keyStr} = ${valueLines[0].trim()}`);
        } else {
          lines.push(`${pad(level + 1)}${keyStr} = ${valueLines[0].trim()}`);
          lines.push(...valueLines.slice(1));
        }
      });
      lines.push(`${indent}}`);
      return lines;
    }

    lines.push(`${indent}${String(value)}`);
    return lines;
  };

  // 计算两个版本之间的差异
  const calculateDiff = (oldConfig: any, newConfig: any): DiffField[] => {
    const fields: DiffField[] = [];
    const allKeys = new Set([...Object.keys(oldConfig), ...Object.keys(newConfig)]);
    
    allKeys.forEach(key => {
      const oldValue = oldConfig[key];
      const newValue = newConfig[key];
      
      const oldExists = key in oldConfig;
      const newExists = key in newConfig;
      
      if (!oldExists && newExists) {
        // 新增字段
        fields.push({ field: key, type: 'added', newValue });
      } else if (oldExists && !newExists) {
        // 删除字段
        fields.push({ field: key, type: 'removed', oldValue });
      } else if (JSON.stringify(oldValue) !== JSON.stringify(newValue)) {
        // 修改字段
        fields.push({ field: key, type: 'modified', oldValue, newValue });
      } else {
        // 未变更字段
        fields.push({ field: key, type: 'unchanged', oldValue, newValue });
      }
    });
    
    // 排序：变更的字段在前，未变更的在后
    return fields.sort((a, b) => {
      const order = { added: 1, removed: 2, modified: 3, unchanged: 4 };
      return order[a.type] - order[b.type];
    });
  };

  // 格式化值显示
  const formatValue = (fieldName: string, value: any): string => {
    if (value === null || value === undefined) return '';

    // 对 typejsonstring 字段使用 jsonencode 格式
    if (isJsonStringField(fieldName)) {
      return formatJsonStringAsHCL(value);
    }

    if (typeof value === 'object') {
      if (isV3) {
        try {
          return jsonToHCL(value, { moduleName: 'diff', skipDefaults: false });
        } catch {
          return JSON.stringify(value, null, 2);
        }
      }
      return JSON.stringify(value, null, 2);
    }
    return String(value);
  };

  // 逐行对比两个 JSON 字符串的差异
  const computeLineDiff = (oldLines: string[], newLines: string[]): Array<{type: 'added' | 'removed' | 'unchanged', content: string}> => {
    const result: Array<{type: 'added' | 'removed' | 'unchanged', content: string}> = [];
    
    // 简单的 LCS 算法
    const m = oldLines.length;
    const n = newLines.length;
    const dp: number[][] = Array(m + 1).fill(null).map(() => Array(n + 1).fill(0));
    
    for (let i = 1; i <= m; i++) {
      for (let j = 1; j <= n; j++) {
        if (oldLines[i - 1] === newLines[j - 1]) {
          dp[i][j] = dp[i - 1][j - 1] + 1;
        } else {
          dp[i][j] = Math.max(dp[i - 1][j], dp[i][j - 1]);
        }
      }
    }
    
    // 回溯构建 diff
    let i = m, j = n;
    while (i > 0 || j > 0) {
      if (i > 0 && j > 0 && oldLines[i - 1] === newLines[j - 1]) {
        result.unshift({ type: 'unchanged', content: oldLines[i - 1] });
        i--; j--;
      } else if (j > 0 && (i === 0 || dp[i][j - 1] >= dp[i - 1][j])) {
        result.unshift({ type: 'added', content: newLines[j - 1] });
        j--;
      } else if (i > 0) {
        result.unshift({ type: 'removed', content: oldLines[i - 1] });
        i--;
      }
    }
    
    return result;
  };

  // 获取diff图标和颜色
  const getDiffIcon = (type: string) => {
    switch (type) {
      case 'added':
        return '+';
      case 'removed':
        return '-';
      case 'modified':
        return '~';
      default:
        return '=';
    }
  };

  if (!isOpen) return null;

  const changedFields = diffFields.filter(f => f.type !== 'unchanged');
  const unchangedFields = diffFields.filter(f => f.type === 'unchanged');

  return (
    <div className={styles.overlay} onClick={onClose}>
      <div className={styles.modal} onClick={(e) => e.stopPropagation()}>
        <div className={styles.header}>
          <h2 className={styles.title}>版本对比</h2>
          <button className={styles.closeButton} onClick={onClose}>
            ×
          </button>
        </div>

        <div className={styles.versionSelector}>
          <div className={styles.selectorGroup}>
            <label className={styles.selectorLabel}>From:</label>
            <select
              className={styles.versionSelect}
              value={fromVersion}
              onChange={(e) => setFromVersion(parseInt(e.target.value))}
            >
              {allVersions.map((v) => (
                <option key={v.id} value={v.version}>
                  v{v.version} {v.change_summary ? `- ${v.change_summary}` : ''}
                </option>
              ))}
            </select>
          </div>
          
          <span className={styles.arrow}>→</span>
          
          <div className={styles.selectorGroup}>
            <label className={styles.selectorLabel}>To:</label>
            <select
              className={styles.versionSelect}
              value={toVersion}
              onChange={(e) => setToVersion(parseInt(e.target.value))}
            >
              {allVersions.map((v) => (
                <option key={v.id} value={v.version}>
                  v{v.version} {v.is_latest ? '(Latest)' : ''} {v.change_summary ? `- ${v.change_summary}` : ''}
                </option>
              ))}
            </select>
          </div>
        </div>

        <div className={styles.body}>
          {loading ? (
            <div className={styles.loading}>
              <div className={styles.spinner}></div>
              <span>加载中...</span>
            </div>
          ) : (
            <>
              <div className={styles.diffSummary}>
                <span className={styles.summaryItem}>
                  <span className={styles.added}>+{changedFields.filter(f => f.type === 'added').length}</span> 新增
                </span>
                <span className={styles.summaryItem}>
                  <span className={styles.removed}>-{changedFields.filter(f => f.type === 'removed').length}</span> 删除
                </span>
                <span className={styles.summaryItem}>
                  <span className={styles.modified}>~{changedFields.filter(f => f.type === 'modified').length}</span> 修改
                </span>
                <span className={styles.summaryItem}>
                  ={unchangedFields.length} 未变更
                </span>
              </div>

              <div className={styles.diffContent}>
                {/* 变更的字段 */}
                {changedFields.map((field) => (
                  <div key={field.field} className={`${styles.diffField} ${styles[field.type]}`}>
                    <div className={styles.fieldHeader}>
                      <span className={styles.diffIcon}>{getDiffIcon(field.type)}</span>
                      <span className={styles.fieldName}>{field.field}</span>
                      <span className={styles.fieldType}>{field.type}</span>
                    </div>
                    <div className={styles.fieldContent}>
                      {field.type === 'removed' && (
                        <pre className={styles.removedValue}>
                          <code>{formatValue(field.field, field.oldValue)}</code>
                        </pre>
                      )}
                      {field.type === 'added' && (
                        <pre className={styles.addedValue}>
                          <code>{formatValue(field.field, field.newValue)}</code>
                        </pre>
                      )}
                      {field.type === 'modified' && (
                        <>
                          {/* 对 typejsonstring 字段进行逐行对比 */}
                          {isJsonStringField(field.field) ? (
                            <div className={styles.lineDiffContainer}>
                              {computeLineDiff(
                                formatValue(field.field, field.oldValue).split('\n'),
                                formatValue(field.field, field.newValue).split('\n')
                              ).map((line, idx) => (
                                <div
                                  key={idx}
                                  className={`${styles.lineDiff} ${
                                    line.type === 'added' ? styles.lineAdded :
                                    line.type === 'removed' ? styles.lineRemoved :
                                    styles.lineUnchanged
                                  }`}
                                >
                                  <span className={styles.linePrefix}>
                                    {line.type === 'added' ? '+' : line.type === 'removed' ? '-' : ' '}
                                  </span>
                                  <code>{line.content}</code>
                                </div>
                              ))}
                            </div>
                          ) : (
                            <>
                              <pre className={styles.removedValue}>
                                <code>{formatValue(field.field, field.oldValue)}</code>
                              </pre>
                              <pre className={styles.addedValue}>
                                <code>{formatValue(field.field, field.newValue)}</code>
                              </pre>
                            </>
                          )}
                        </>
                      )}
                    </div>
                  </div>
                ))}

                {/* 未变更的字段（可折叠） */}
                {unchangedFields.length > 0 && (
                  <div className={styles.unchangedSection}>
                    <button
                      className={styles.toggleUnchanged}
                      onClick={() => setShowUnchanged(!showUnchanged)}
                    >
                      {showUnchanged ? '▼' : '▶'} {unchangedFields.length} 个未变更字段
                    </button>
                    {showUnchanged && (
                      <div className={styles.unchangedFields}>
                        {unchangedFields.map((field) => (
                          <div key={field.field} className={styles.unchangedField}>
                            <span className={styles.fieldName}>{field.field}</span>
                            <span className={styles.unchangedValue}>
                              {formatValue(field.field, field.oldValue)}
                            </span>
                          </div>
                        ))}
                      </div>
                    )}
                  </div>
                )}
              </div>
            </>
          )}
        </div>

        <div className={styles.footer}>
          <button className={styles.btnClose} onClick={onClose}>
            关闭
          </button>
        </div>
      </div>
    </div>
  );
};

export default ResourceVersionDiff;
