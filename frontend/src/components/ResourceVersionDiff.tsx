import React, { useState, useEffect } from 'react';
import api from '../services/api';
import { useToast } from '../contexts/ToastContext';
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
  const { showToast } = useToast();

  useEffect(() => {
    if (isOpen) {
      loadVersionsAndCompare();
    }
  }, [isOpen, fromVersion, toVersion]);

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
  const formatValue = (value: any): string => {
    if (value === null || value === undefined) return '';
    if (typeof value === 'object') {
      return JSON.stringify(value, null, 2);
    }
    return String(value);
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
                          <code>{formatValue(field.oldValue)}</code>
                        </pre>
                      )}
                      {field.type === 'added' && (
                        <pre className={styles.addedValue}>
                          <code>{formatValue(field.newValue)}</code>
                        </pre>
                      )}
                      {field.type === 'modified' && (
                        <>
                          <pre className={styles.removedValue}>
                            <code>{formatValue(field.oldValue)}</code>
                          </pre>
                          <pre className={styles.addedValue}>
                            <code>{formatValue(field.newValue)}</code>
                          </pre>
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
                              {formatValue(field.oldValue)}
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
