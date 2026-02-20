import React, { useState, useMemo } from 'react';
import styles from './PlanCompleteView.module.css';

interface ResourceChange {
  id: number;
  resource_address: string;
  resource_type: string;
  resource_name: string;
  module_address: string;
  action: string;
  changes_before: Record<string, any>;
  changes_after: Record<string, any>;
  apply_status: string;
}

interface OutputChange {
  name: string;
  action: string;
  before: any;
  after: any;
  after_unknown: boolean;
  sensitive: boolean;
}

interface ActionInvocation {
  name: string;
  type: string;
  address: string;
  config_values?: Record<string, any>;
  config_unknown?: Record<string, any>;
  provider_name?: string;
  lifecycle_action_trigger?: {
    actions_list_index: number;
    action_trigger_event: string;
    action_trigger_block_index: number;
    triggering_resource_address: string;
  };
}

// Action 资源定义（来自 configuration.root_module.module_calls.*.module.actions）
interface ActionResource {
  name: string;
  type: string;
  address: string;
  full_address?: string;
  module_address?: string;
  provider_config_key?: string;
}

interface Props {
  resources: ResourceChange[];
  summary: {
    add: number;
    change: number;
    destroy: number;
  };
  outputChanges?: OutputChange[];
  actionInvocations?: ActionInvocation[];
  actions?: ActionResource[];
}

// 判断值是否为空或无意义
const isEmptyValue = (value: any): boolean => {
  if (value === null || value === undefined) return true;
  if (value === '') return true;
  if (Array.isArray(value) && value.length === 0) return true;
  if (typeof value === 'object' && !Array.isArray(value) && Object.keys(value).length === 0) return true;
  return false;
};

// 判断是否为复杂对象
const isComplexObject = (value: any): boolean => {
  if (value === null || value === undefined) return false;
  if (Array.isArray(value)) return value.length > 0;
  if (typeof value === 'object') return Object.keys(value).length > 0;
  return false;
};

// 格式化简单值
const formatSimpleValue = (value: any): string => {
  if (value === null) return 'null';
  if (value === undefined) return 'undefined';
  if (value === '') return '""';
  if (typeof value === 'boolean') return value ? 'true' : 'false';
  if (typeof value === 'number') return String(value);
  if (typeof value === 'string') return `"${value}"`;
  if (Array.isArray(value)) return JSON.stringify(value);
  if (typeof value === 'object') return JSON.stringify(value);
  return String(value);
};

const PlanCompleteView: React.FC<Props> = ({ resources, outputChanges = [], actionInvocations = [], actions = [] }) => {
  const [expandedResources, setExpandedResources] = useState<Set<number>>(new Set());
  const [showUnchanged, setShowUnchanged] = useState<Set<number>>(new Set());
  const [searchTerm, setSearchTerm] = useState('');
  const [selectedActions, setSelectedActions] = useState<Set<string>>(new Set());
  const [showActionFilter, setShowActionFilter] = useState(false);
  const [copySuccess, setCopySuccess] = useState<string | null>(null);
  const [outputsExpanded, setOutputsExpanded] = useState(false);
  const [actionsExpanded, setActionsExpanded] = useState(true);
  const [expandedActionIndices, setExpandedActionIndices] = useState<Set<number>>(new Set());

  // 计算每个action的数量
  const actionCounts = useMemo(() => ({
    create: resources.filter(r => r.action === 'create').length,
    update: resources.filter(r => r.action === 'update').length,
    delete: resources.filter(r => r.action === 'delete').length,
    replace: resources.filter(r => r.action === 'replace').length,
  }), [resources]);

  // 创建从触发资源地址到 action invocations 的映射
  const triggerToActionsMap = useMemo(() => {
    const map = new Map<string, ActionInvocation[]>();
    actionInvocations.forEach(action => {
      const triggerResource = action.lifecycle_action_trigger?.triggering_resource_address;
      if (triggerResource) {
        const existing = map.get(triggerResource) || [];
        existing.push(action);
        map.set(triggerResource, existing);
      }
    });
    return map;
  }, [actionInvocations]);

  // 过滤资源
  const filteredResources = useMemo(() => {
    return resources.filter(resource => {
      const matchesSearch = !searchTerm || 
        resource.resource_address.toLowerCase().includes(searchTerm.toLowerCase()) ||
        resource.resource_type.toLowerCase().includes(searchTerm.toLowerCase());
      const matchesAction = selectedActions.size === 0 || selectedActions.has(resource.action);
      return matchesSearch && matchesAction;
    });
  }, [resources, searchTerm, selectedActions]);

  // 点击外部关闭下拉菜单
  React.useEffect(() => {
    const handleClickOutside = (event: MouseEvent) => {
      const target = event.target as HTMLElement;
      if (!target.closest(`.${styles.actionFilterContainer}`)) {
        setShowActionFilter(false);
      }
    };
    if (showActionFilter) {
      document.addEventListener('click', handleClickOutside);
      return () => document.removeEventListener('click', handleClickOutside);
    }
  }, [showActionFilter]);

  const toggleResource = (id: number) => {
    setExpandedResources(prev => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  const toggleUnchanged = (id: number) => {
    setShowUnchanged(prev => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  const toggleAction = (action: string) => {
    setSelectedActions(prev => {
      const next = new Set(prev);
      if (next.has(action)) next.delete(action);
      else next.add(action);
      return next;
    });
  };

  const copyToClipboard = async (text: string, label: string) => {
    try {
      await navigator.clipboard.writeText(text);
      setCopySuccess(`Copied ${label}`);
      setTimeout(() => setCopySuccess(null), 2000);
    } catch (err) {
      console.error('Failed to copy:', err);
    }
  };

  // 计算变更的字段
  const computeChanges = (before: Record<string, any>, after: Record<string, any>) => {
    const changed: Array<{ key: string; before: any; after: any; type: 'add' | 'remove' | 'modify' }> = [];
    const unchanged: string[] = [];
    const allKeys = new Set([...Object.keys(before || {}), ...Object.keys(after || {})]);

    allKeys.forEach(key => {
      const beforeVal = before?.[key];
      const afterVal = after?.[key];
      const beforeEmpty = isEmptyValue(beforeVal);
      const afterEmpty = isEmptyValue(afterVal);

      if (beforeEmpty && afterEmpty) return;

      if (JSON.stringify(beforeVal) === JSON.stringify(afterVal)) {
        unchanged.push(key);
      } else if (beforeEmpty && !afterEmpty) {
        changed.push({ key, before: beforeVal, after: afterVal, type: 'add' });
      } else if (!beforeEmpty && afterEmpty) {
        changed.push({ key, before: beforeVal, after: afterVal, type: 'remove' });
      } else {
        changed.push({ key, before: beforeVal, after: afterVal, type: 'modify' });
      }
    });

    return { changed, unchanged };
  };

  // 渲染值
  const renderValue = (value: any, variant: 'create' | 'delete' | 'before' | 'after' | 'neutral' = 'neutral') => {
    if (isEmptyValue(value) && (variant === 'create' || variant === 'after')) {
      return <span className={styles.knownAfterApply}>Known after apply</span>;
    }

    const className = {
      create: styles.valueCreate,
      delete: styles.valueDelete,
      before: styles.valueBefore,
      after: styles.valueAfter,
      neutral: styles.valueNeutral,
    }[variant];

    return <span className={className}>{formatSimpleValue(value)}</span>;
  };

  // 渲染嵌套对象的变更
  const renderNestedChanges = (
    key: string,
    beforeObj: Record<string, any>,
    afterObj: Record<string, any>,
    action: 'create' | 'delete' | 'update'
  ) => {
    const allSubKeys = new Set([...Object.keys(beforeObj || {}), ...Object.keys(afterObj || {})]);
    const subChanges: Array<{ subKey: string; before: any; after: any; type: 'add' | 'remove' | 'modify' | 'unchanged' }> = [];

    allSubKeys.forEach(subKey => {
      const beforeVal = beforeObj?.[subKey];
      const afterVal = afterObj?.[subKey];
      const beforeEmpty = isEmptyValue(beforeVal);
      const afterEmpty = isEmptyValue(afterVal);

      if (beforeEmpty && afterEmpty) return;

      if (JSON.stringify(beforeVal) === JSON.stringify(afterVal)) {
        subChanges.push({ subKey, before: beforeVal, after: afterVal, type: 'unchanged' });
      } else if (beforeEmpty) {
        subChanges.push({ subKey, before: beforeVal, after: afterVal, type: 'add' });
      } else if (afterEmpty) {
        subChanges.push({ subKey, before: beforeVal, after: afterVal, type: 'remove' });
      } else {
        subChanges.push({ subKey, before: beforeVal, after: afterVal, type: 'modify' });
      }
    });

    // 对于 create/delete，只显示有值的
    const displayChanges = action === 'update' 
      ? subChanges.filter(c => c.type !== 'unchanged')
      : subChanges;

    if (displayChanges.length === 0) return null;

    return (
      <div key={key} className={styles.nestedGroup}>
        <div className={styles.nestedHeader}>{key}:</div>
        <div className={styles.nestedContent}>
          {displayChanges.map(({ subKey, before, after, type }) => (
            <div key={subKey} className={styles.nestedRow}>
              <span className={`${styles.changeIcon} ${styles[`icon${type.charAt(0).toUpperCase() + type.slice(1)}`]}`}>
                {type === 'add' ? '+' : type === 'remove' ? '−' : type === 'modify' ? '~' : ' '}
              </span>
              <span className={styles.nestedKey}>{subKey} =</span>
              {action === 'create' ? (
                renderValue(after, 'create')
              ) : action === 'delete' ? (
                renderValue(before, 'delete')
              ) : type === 'add' ? (
                renderValue(after, 'create')
              ) : type === 'remove' ? (
                renderValue(before, 'delete')
              ) : (
                <span className={styles.valueComparison}>
                  {renderValue(before, 'before')}
                  <span className={styles.arrow}>→</span>
                  {renderValue(after, 'after')}
                </span>
              )}
            </div>
          ))}
        </div>
      </div>
    );
  };

  // 渲染 CREATE 资源的属性
  const renderCreateBody = (resource: ResourceChange) => {
    const after = resource.changes_after || {};
    const entries = Object.entries(after).filter(([_, v]) => !isEmptyValue(v));
    
    if (entries.length === 0) {
      return null;
    }

    // 分离简单值和复杂对象
    const simpleEntries = entries.filter(([_, v]) => !isComplexObject(v));
    const complexEntries = entries.filter(([_, v]) => isComplexObject(v) && typeof v === 'object' && !Array.isArray(v));
    const arrayEntries = entries.filter(([_, v]) => Array.isArray(v));

    return (
      <>
        <div className={styles.sectionLabel}>Resource will be created:</div>
        
        {/* 简单属性 - 紧凑的表格布局 */}
        {simpleEntries.length > 0 && (
          <div className={styles.simpleAttrsGrid}>
            {simpleEntries.map(([key, value]) => (
              <div key={key} className={styles.simpleAttrRow}>
                <span className={styles.attrIcon}>+</span>
                <span className={styles.attrKey}>{key}:</span>
                {renderValue(value, 'create')}
              </div>
            ))}
          </div>
        )}

        {/* 复杂对象 - 嵌套展示 */}
        {complexEntries.map(([key, value]) => 
          renderNestedChanges(key, {}, value as Record<string, any>, 'create')
        )}

        {/* 数组 */}
        {arrayEntries.map(([key, value]) => (
          <div key={key} className={styles.simpleAttrRow}>
            <span className={styles.attrIcon}>+</span>
            <span className={styles.attrKey}>{key}:</span>
            {renderValue(value, 'create')}
          </div>
        ))}
      </>
    );
  };

  // 渲染 DELETE 资源的属性
  const renderDeleteBody = (resource: ResourceChange) => {
    const before = resource.changes_before || {};
    const entries = Object.entries(before).filter(([_, v]) => !isEmptyValue(v));
    
    if (entries.length === 0) {
      return <div className={styles.emptyMessage}>No attributes to display</div>;
    }

    const simpleEntries = entries.filter(([_, v]) => !isComplexObject(v));
    const complexEntries = entries.filter(([_, v]) => isComplexObject(v) && typeof v === 'object' && !Array.isArray(v));
    const arrayEntries = entries.filter(([_, v]) => Array.isArray(v));

    return (
      <>
        <div className={`${styles.sectionLabel} ${styles.sectionLabelDelete}`}>Resource will be destroyed:</div>
        
        {simpleEntries.length > 0 && (
          <div className={styles.simpleAttrsGrid}>
            {simpleEntries.map(([key, value]) => (
              <div key={key} className={styles.simpleAttrRow}>
                <span className={`${styles.attrIcon} ${styles.attrIconDelete}`}>−</span>
                <span className={styles.attrKey}>{key}:</span>
                {renderValue(value, 'delete')}
              </div>
            ))}
          </div>
        )}

        {complexEntries.map(([key, value]) => 
          renderNestedChanges(key, value as Record<string, any>, {}, 'delete')
        )}

        {arrayEntries.map(([key, value]) => (
          <div key={key} className={styles.simpleAttrRow}>
            <span className={`${styles.attrIcon} ${styles.attrIconDelete}`}>−</span>
            <span className={styles.attrKey}>{key}:</span>
            {renderValue(value, 'delete')}
          </div>
        ))}
      </>
    );
  };

  // 渲染 UPDATE/REPLACE 资源的属性
  const renderUpdateBody = (resource: ResourceChange) => {
    const { changed, unchanged } = computeChanges(resource.changes_before, resource.changes_after);
    
    if (changed.length === 0) {
      return <div className={styles.emptyMessage}>No changes detected</div>;
    }

    // 分离简单变更和复杂对象变更
    const simpleChanges = changed.filter(c => 
      !isComplexObject(c.before) && !isComplexObject(c.after)
    );
    const complexChanges = changed.filter(c => 
      (isComplexObject(c.before) || isComplexObject(c.after)) &&
      (typeof c.before === 'object' && !Array.isArray(c.before)) ||
      (typeof c.after === 'object' && !Array.isArray(c.after))
    );
    const arrayChanges = changed.filter(c =>
      Array.isArray(c.before) || Array.isArray(c.after)
    );

    return (
      <>
        <div className={styles.sectionLabel}>Changed fields:</div>
        
        {/* 简单属性变更 */}
        {simpleChanges.length > 0 && (
          <div className={styles.changesTable}>
            {simpleChanges.map(({ key, before, after, type }) => (
              <div key={key} className={styles.changeRow}>
                <span className={`${styles.changeIcon} ${styles[`icon${type.charAt(0).toUpperCase() + type.slice(1)}`]}`}>
                  {type === 'add' ? '+' : type === 'remove' ? '−' : '~'}
                </span>
                <span className={styles.changeKey}>{key} =</span>
                {type === 'add' ? (
                  renderValue(after, 'create')
                ) : type === 'remove' ? (
                  renderValue(before, 'delete')
                ) : (
                  <span className={styles.valueComparison}>
                    {renderValue(before, 'before')}
                    <span className={styles.arrow}>→</span>
                    {renderValue(after, 'after')}
                  </span>
                )}
              </div>
            ))}
          </div>
        )}

        {/* 复杂对象变更 */}
        {complexChanges.map(({ key, before, after }) => 
          renderNestedChanges(
            key, 
            (before as Record<string, any>) || {}, 
            (after as Record<string, any>) || {}, 
            'update'
          )
        )}

        {/* 数组变更 */}
        {arrayChanges.map(({ key, before, after, type }) => (
          <div key={key} className={styles.changeRow}>
            <span className={`${styles.changeIcon} ${styles[`icon${type.charAt(0).toUpperCase() + type.slice(1)}`]}`}>
              {type === 'add' ? '+' : type === 'remove' ? '−' : '~'}
            </span>
            <span className={styles.changeKey}>{key} =</span>
            {type === 'add' ? (
              renderValue(after, 'create')
            ) : type === 'remove' ? (
              renderValue(before, 'delete')
            ) : (
              <span className={styles.valueComparison}>
                {renderValue(before, 'before')}
                <span className={styles.arrow}>→</span>
                {renderValue(after, 'after')}
              </span>
            )}
          </div>
        ))}

        {/* 未变更属性折叠 */}
        {unchanged.length > 0 && (
          <>
            <div 
              className={styles.unchangedToggle} 
              onClick={() => toggleUnchanged(resource.id)}
            >
              <span className={styles.toggleIcon}>
                {showUnchanged.has(resource.id) ? '▼' : '▶'}
              </span>
              {showUnchanged.has(resource.id) 
                ? `Hide ${unchanged.length} unchanged elements`
                : `Show ${unchanged.length} unchanged elements`
              }
            </div>
            
            {showUnchanged.has(resource.id) && (
              <div className={styles.unchangedList}>
                {unchanged.map(key => (
                  <div key={key} className={styles.unchangedRow}>
                    <span className={styles.unchangedKey}>{key}:</span>
                    <span className={styles.unchangedValue}>
                      {formatSimpleValue(resource.changes_before[key])}
                    </span>
                  </div>
                ))}
              </div>
            )}
          </>
        )}
      </>
    );
  };

  const getActionConfig = (action: string) => {
    switch (action) {
      case 'create': return { icon: '+', label: 'CREATE', className: styles.actionCreate };
      case 'update': return { icon: '~', label: 'UPDATE', className: styles.actionUpdate };
      case 'delete': return { icon: '−', label: 'DELETE', className: styles.actionDelete };
      case 'replace': return { icon: '±', label: 'REPLACE', className: styles.actionReplace };
      default: return { icon: '?', label: action.toUpperCase(), className: '' };
    }
  };

  return (
    <div className={styles.planComplete}>
      {copySuccess && <div className={styles.copyToast}>{copySuccess}</div>}
      
      {/* 过滤栏 */}
      <div className={styles.filterBar}>
        <div className={styles.filterRow}>
          <div className={styles.searchBox}>
            <svg className={styles.searchIcon} width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
              <circle cx="11" cy="11" r="8"></circle>
              <path d="M21 21l-4.35-4.35"></path>
            </svg>
            <input
              type="text"
              placeholder="Filter resources by address..."
              value={searchTerm}
              onChange={(e) => setSearchTerm(e.target.value)}
              className={styles.searchInput}
            />
            {searchTerm && (
              <button onClick={() => setSearchTerm('')} className={styles.clearButton}>✕</button>
            )}
          </div>

          <div className={styles.actionFilterContainer}>
            <button
              className={styles.actionFilterButton}
              onClick={() => setShowActionFilter(!showActionFilter)}
            >
              <span className={styles.filterIcon}>☰</span>
              Filter
              {selectedActions.size > 0 && (
                <span className={styles.filterCount}>{selectedActions.size}</span>
              )}
              <span className={styles.chevron}>{showActionFilter ? '▲' : '▼'}</span>
            </button>

            {showActionFilter && (
              <div className={styles.actionFilterDropdown}>
                {['create', 'update', 'delete', 'replace'].map(action => (
                  <label key={action} className={styles.actionFilterItem}>
                    <input 
                      type="checkbox" 
                      checked={selectedActions.has(action)} 
                      onChange={() => toggleAction(action)} 
                    />
                    <span className={styles.actionFilterLabel}>
                      <span className={styles[`icon${action.charAt(0).toUpperCase() + action.slice(1)}`]}>
                        {getActionConfig(action).icon}
                      </span>
                      {action.charAt(0).toUpperCase() + action.slice(1)} ({actionCounts[action as keyof typeof actionCounts]})
                    </span>
                  </label>
                ))}
              </div>
            )}
          </div>
        </div>

        {(searchTerm || selectedActions.size > 0) && (
          <div className={styles.filterResult}>
            Showing {filteredResources.length} of {resources.length} resources
            <button
              onClick={() => { setSearchTerm(''); setSelectedActions(new Set()); }}
              className={styles.clearFiltersButton}
            >
              Clear filters
            </button>
          </div>
        )}
      </div>

      {/* 资源列表 */}
      <div className={styles.resourceList}>
        {filteredResources.map((resource) => {
          const isExpanded = expandedResources.has(resource.id);
          const config = getActionConfig(resource.action);
          // 检查这个资源是否会触发 actions
          const triggeredActions = triggerToActionsMap.get(resource.resource_address) || [];

          return (
            <div key={resource.id} className={`${styles.resourceCard} ${config.className}`}>
              <div className={styles.resourceHeader} onClick={() => toggleResource(resource.id)}>
                <div className={styles.resourceHeaderLeft}>
                  <span className={styles.expandIcon}>{isExpanded ? '▼' : '▶'}</span>
                  <span className={`${styles.actionIcon} ${config.className}`}>{config.icon}</span>
                  <span className={styles.resourceAddress}>{resource.resource_address}</span>
                  <button
                    className={styles.copyButton}
                    onClick={(e) => { e.stopPropagation(); copyToClipboard(resource.resource_address, 'address'); }}
                    title="Copy resource address"
                  >
                    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                      <rect x="9" y="9" width="13" height="13" rx="2" ry="2"></rect>
                      <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"></path>
                    </svg>
                  </button>
                  {/* 显示触发的 actions */}
                  {triggeredActions.length > 0 && (
                    <span className={styles.triggersActionIndicator} title={`Triggers ${triggeredActions.length} action(s)`}>
                      <svg width="16" height="16" viewBox="0 0 24 16" fill="none" stroke="currentColor" strokeWidth="2">
                        <path d="M0 8h16M12 4l4 4-4 4" />
                      </svg>
                      <span className={styles.triggersActionBadge}>
                        {triggeredActions.map(a => a.type).join(', ')}
                      </span>
                    </span>
                  )}
                </div>
                <div className={styles.resourceHeaderRight}>
                  <span className={styles.resourceTypeTag}>{resource.resource_type}</span>
                  <span className={`${styles.actionBadge} ${config.className}`}>{config.label}</span>
                </div>
              </div>

              {isExpanded && (
                <div className={styles.resourceBody}>
                  {resource.action === 'create' && renderCreateBody(resource)}
                  {resource.action === 'delete' && renderDeleteBody(resource)}
                  {(resource.action === 'update' || resource.action === 'replace') && renderUpdateBody(resource)}
                  
                  {/* 显示触发的 actions 详情 */}
                  {triggeredActions.length > 0 && (
                    <div className={styles.triggeredActionsSection}>
                      <div className={styles.triggeredActionsLabel}>
                        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" style={{ marginRight: '6px', verticalAlign: 'middle' }}>
                          <polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2"></polygon>
                        </svg>
                        Triggers Actions:
                      </div>
                      {triggeredActions.map((action, idx) => {
                        const triggerEvent = action.lifecycle_action_trigger?.action_trigger_event || 'Unknown';
                        const formatTriggerEvent = (event: string) => {
                          switch (event) {
                            case 'AfterCreate': return 'After Create';
                            case 'AfterUpdate': return 'After Update';
                            case 'BeforeDestroy': return 'Before Destroy';
                            default: return event;
                          }
                        };
                        return (
                          <div key={idx} className={styles.triggeredActionItem}>
                            <span className={styles.triggeredActionArrow}>→</span>
                            <span className={styles.triggeredActionAddress} title={action.address}>{action.address}</span>
                            <button
                              className={styles.copyButton}
                              onClick={(e) => { e.stopPropagation(); copyToClipboard(action.address, 'action address'); }}
                              title="Copy action address"
                            >
                              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                                <rect x="9" y="9" width="13" height="13" rx="2" ry="2"></rect>
                                <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"></path>
                              </svg>
                            </button>
                            <span className={styles.triggeredActionType}>{action.type}</span>
                            <span className={styles.triggeredActionEvent}>{formatTriggerEvent(triggerEvent)}</span>
                          </div>
                        );
                      })}
                    </div>
                  )}
                </div>
              )}
            </div>
          );
        })}
      </div>

      {/* Action Invocations (Terraform 1.14+ 新特性) */}
      {actionInvocations.length > 0 && (
        <div className={styles.actionInvocationsSection}>
          <div className={styles.actionInvocationsHeader} onClick={() => setActionsExpanded(!actionsExpanded)}>
            <span className={styles.expandIcon}>{actionsExpanded ? '▼' : '▶'}</span>
            <span className={styles.actionInvocationsTitle}>
              <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" style={{ marginRight: '6px', verticalAlign: 'middle' }}>
                <polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2"></polygon>
              </svg>
              Action Invocations
            </span>
            <span className={styles.actionInvocationsCount}>{actionInvocations.length}</span>
            <span className={styles.actionInvocationsBadge}>Terraform 1.14+</span>
          </div>
          {actionsExpanded && (
            <div className={styles.actionInvocationsList}>
              {actionInvocations.map((action, index) => {
                const isExpanded = expandedActionIndices.has(index);
                const triggerEvent = action.lifecycle_action_trigger?.action_trigger_event || 'Unknown';
                const triggerResource = action.lifecycle_action_trigger?.triggering_resource_address || '';
                
                // 格式化触发事件名称
                const formatTriggerEvent = (event: string) => {
                  switch (event) {
                    case 'AfterCreate': return 'After Create';
                    case 'AfterUpdate': return 'After Update';
                    case 'BeforeDestroy': return 'Before Destroy';
                    default: return event;
                  }
                };
                
                return (
                  <div key={index} className={styles.actionInvocationItem}>
                    <div 
                      className={styles.actionInvocationHeader}
                      onClick={() => {
                        setExpandedActionIndices(prev => {
                          const next = new Set(prev);
                          if (next.has(index)) next.delete(index);
                          else next.add(index);
                          return next;
                        });
                      }}
                    >
                      <div className={styles.actionInvocationLeft}>
                        <span className={styles.expandIcon}>{isExpanded ? '▼' : '▶'}</span>
                        <span className={styles.actionInvocationAddress}>{action.address}</span>
                        <button
                          className={styles.copyButton}
                          onClick={(e) => { e.stopPropagation(); copyToClipboard(action.address, 'action address'); }}
                          title="Copy action address"
                        >
                          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                            <rect x="9" y="9" width="13" height="13" rx="2" ry="2"></rect>
                            <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"></path>
                          </svg>
                        </button>
                      </div>
                      <div className={styles.actionInvocationRight}>
                        <span className={styles.actionInvocationTypeTag}>{action.type}</span>
                        <span className={styles.actionInvocationTriggerBadge}>{formatTriggerEvent(triggerEvent)}</span>
                      </div>
                    </div>
                    
                    {isExpanded && (
                      <div className={styles.actionInvocationBody}>
                        {/* 触发关系 */}
                        {triggerResource && (
                          <div className={styles.actionTriggerRelation}>
                            <div className={styles.triggerRelationLabel}>Triggered by:</div>
                            <div className={styles.triggerRelationFlow}>
                              <span className={styles.triggerResourceAddress}>{triggerResource}</span>
                            </div>
                          </div>
                        )}
                        
                        {/* Action 配置值 */}
                        {action.config_values && Object.keys(action.config_values).length > 0 && (
                          <div className={styles.actionConfigSection}>
                            <div className={styles.actionConfigLabel}>Configuration:</div>
                            <div className={styles.actionConfigGrid}>
                              {Object.entries(action.config_values).map(([key, value]) => {
                                // 跳过空值和空数组
                                if (value === null || (Array.isArray(value) && value.length === 0)) {
                                  // 检查是否在 config_unknown 中
                                  if (action.config_unknown?.[key]) {
                                    return (
                                      <div key={key} className={styles.actionConfigRow}>
                                        <span className={styles.actionConfigKey}>{key}:</span>
                                        <span className={styles.knownAfterApply}>(known after apply)</span>
                                      </div>
                                    );
                                  }
                                  return null;
                                }
                                return (
                                  <div key={key} className={styles.actionConfigRow}>
                                    <span className={styles.actionConfigKey}>{key}:</span>
                                    <span className={styles.actionConfigValue}>{formatSimpleValue(value)}</span>
                                  </div>
                                );
                              })}
                            </div>
                          </div>
                        )}
                        
                        {/* Provider 信息 */}
                        {action.provider_name && (
                          <div className={styles.actionProviderInfo}>
                            <span className={styles.actionProviderLabel}>Provider:</span>
                            <span className={styles.actionProviderValue}>{action.provider_name}</span>
                          </div>
                        )}
                      </div>
                    )}
                  </div>
                );
              })}
            </div>
          )}
        </div>
      )}

      {/* Output Changes */}
      {outputChanges.length > 0 && (() => {
        // 过滤掉没有实际变更的 outputs（before 和 after 相同）
        const actualChanges = outputChanges.filter(output => {
          // no-op 表示没有变更
          if (output.action === 'no-op') return false;
          // 如果 before 和 after 相同，也不显示
          if (JSON.stringify(output.before) === JSON.stringify(output.after)) return false;
          return true;
        });
        
        if (actualChanges.length === 0) return null;
        
        return (
          <div className={styles.outputChangesSection}>
            <div className={styles.outputChangesHeader} onClick={() => setOutputsExpanded(!outputsExpanded)}>
              <span className={styles.expandIcon}>{outputsExpanded ? '▼' : '▶'}</span>
              <span className={styles.outputChangesTitle}>Output Changes</span>
              <span className={styles.outputChangesCount}>{actualChanges.length}</span>
            </div>
            {outputsExpanded && (
              <div className={styles.outputChangesList}>
                {actualChanges.map((output, index) => (
                  <div key={index} className={styles.outputChangeItem}>
                    <div className={styles.outputChangeHeader}>
                      <span className={`${styles.actionIcon} ${styles[`action${output.action.charAt(0).toUpperCase() + output.action.slice(1)}`]}`}>
                        {output.action === 'create' ? '+' : output.action === 'delete' ? '−' : '~'}
                      </span>
                      <span className={styles.outputName}>{output.name}</span>
                      {output.sensitive && <span className={styles.sensitiveTag}>sensitive</span>}
                    </div>
                    <div className={styles.outputChangeValue}>
                      {output.sensitive ? (
                        // Sensitive outputs: always show masked value
                        <span className={styles.sensitiveValue}>(sensitive value)</span>
                      ) : output.action === 'create' ? (
                        output.after_unknown ? (
                          <span className={styles.knownAfterApply}>(known after apply)</span>
                        ) : (
                          renderValue(output.after, 'create')
                        )
                      ) : output.action === 'delete' ? (
                        renderValue(output.before, 'delete')
                      ) : (
                        // Update action
                        <span className={styles.valueComparison}>
                          {renderValue(output.before, 'before')}
                          <span className={styles.arrow}>→</span>
                          {output.after_unknown ? (
                            <span className={styles.knownAfterApply}>(known after apply)</span>
                          ) : (
                            renderValue(output.after, 'after')
                          )}
                        </span>
                      )}
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>
        );
      })()}

      {/* 空状态 */}
      {resources.length === 0 && outputChanges.length === 0 && (
        <div className={styles.noResources}>
          <div className={styles.noResourcesIcon}>
            <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
              <path d="M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2" />
            </svg>
          </div>
          <h3>No Resource Changes Data</h3>
          <p>Unable to load parsed resource changes for this task.</p>
          <p className={styles.noResourcesHint}>
            Possible reasons:<br />
            - Task was created before the feature was implemented<br />
            - Async parsing hasn't completed yet (wait a few seconds)<br />
            - Plan detected no changes
          </p>
        </div>
      )}
    </div>
  );
};

export default PlanCompleteView;
