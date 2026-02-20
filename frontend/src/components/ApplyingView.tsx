import React, { useState, useMemo } from 'react';
import styles from './ApplyingView.module.css';

interface ResourceChange {
  id: number;
  resource_address: string;
  resource_type: string;
  resource_name: string;
  module_address: string;
  action: string;
  apply_status: string;
  apply_started_at?: string;
  apply_completed_at?: string;
  resource_id?: string;
  resource_attributes?: Record<string, any>;
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
  config_resolved?: boolean; // 标记是否已从 state 获取实际值
  provider_name?: string;
  lifecycle_action_trigger?: {
    actions_list_index: number;
    action_trigger_event: string;
    action_trigger_block_index: number;
    triggering_resource_address: string;
  };
}

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
  isApplied?: boolean; // 任务是否已完成 Apply
}

// 格式化 output 值
const formatOutputValue = (value: any): string => {
  if (value === null) return 'null';
  if (value === undefined) return 'undefined';
  if (typeof value === 'string') {
    if (value.length > 50) {
      return `"${value.substring(0, 50)}..."`;
    }
    return `"${value}"`;
  }
  if (typeof value === 'object') {
    return JSON.stringify(value);
  }
  return String(value);
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

const ApplyingView: React.FC<Props> = ({ resources, outputChanges = [], actionInvocations = [], actions = [], isApplied = false }) => {
  const [expandedResources, setExpandedResources] = useState<Set<number>>(new Set());
  const [copySuccess, setCopySuccess] = useState<string | null>(null);
  const [outputsExpanded, setOutputsExpanded] = useState(false);
  const [actionsExpanded, setActionsExpanded] = useState(true);
  const [expandedActionIndices, setExpandedActionIndices] = useState<Set<number>>(new Set());

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

  // 复制到剪贴板
  const copyToClipboard = async (text: string, label: string) => {
    try {
      await navigator.clipboard.writeText(text);
      setCopySuccess(`Copied ${label}`);
      setTimeout(() => setCopySuccess(null), 2000);
    } catch (err) {
      console.error('Failed to copy:', err);
      setCopySuccess('Failed to copy');
      setTimeout(() => setCopySuccess(null), 2000);
    }
  };

  const toggleResource = (id: number) => {
    const newExpanded = new Set(expandedResources);
    if (newExpanded.has(id)) {
      newExpanded.delete(id);
    } else {
      newExpanded.add(id);
    }
    setExpandedResources(newExpanded);
  };

  // 获取状态图标
  const getStatusIcon = (status: string) => {
    switch (status) {
      case 'applying':
        return <span className={styles.statusApplying}>⟳</span>;
      case 'completed':
        return <span className={styles.statusCompleted}>✓</span>;
      case 'failed':
        return <span className={styles.statusFailed}>✗</span>;
      case 'pending':
      default:
        return <span className={styles.statusPending}>○</span>;
    }
  };

  // 获取操作图标
  const getActionIcon = (action: string) => {
    switch (action) {
      case 'create':
        return <span className={styles.actionCreate}>+</span>;
      case 'update':
        return <span className={styles.actionUpdate}>~</span>;
      case 'delete':
        return <span className={styles.actionDelete}>-</span>;
      case 'replace':
        return <span className={styles.actionReplace}>±</span>;
      default:
        return null;
    }
  };

  // 获取状态文本和样式类
  const getStatusTextAndClass = (resource: ResourceChange): { text: string; className: string } => {
    if (resource.apply_status === 'failed') {
      return { text: 'Failed', className: styles.statusTextFailed };
    }

    if (resource.apply_status === 'pending') {
      return { text: 'Pending', className: styles.resourceStatusText };
    }

    if (resource.apply_status === 'applying') {
      const text = resource.action === 'create' ? 'Creating...' 
        : resource.action === 'update' ? 'Modifying...' 
        : 'Destroying...';
      return { text, className: styles.resourceStatusText };
    }

    // completed 状态根据 action 显示不同颜色
    if (resource.apply_status === 'completed') {
      if (resource.action === 'create') {
        return { text: 'Created', className: styles.statusTextCreated };
      } else if (resource.action === 'update') {
        return { text: 'Modified', className: styles.statusTextUpdated };
      } else if (resource.action === 'delete') {
        return { text: 'Destroyed', className: styles.statusTextDestroyed };
      }
    }

    return { text: 'Pending', className: styles.resourceStatusText };
  };

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
    <div className={styles.applyingView}>
      {/* 复制成功提示 */}
      {copySuccess && (
        <div className={styles.copyToast}>
          {copySuccess}
        </div>
      )}

      {/* 资源列表 */}
      <div className={styles.resourceList}>
        {resources.map((resource) => {
          // 检查这个资源是否会触发 actions
          const triggeredActions = triggerToActionsMap.get(resource.resource_address) || [];
          
          return (
            <div key={resource.id} className={styles.resourceItem}>
              <div className={styles.resourceHeader}>
                <div className={styles.resourceStatus}>
                  {getStatusIcon(resource.apply_status)}
                </div>
                <div className={styles.resourceAction}>
                  {getActionIcon(resource.action)}
                </div>
                <div 
                  className={styles.resourceAddress}
                  onClick={() => {
                    // completed状态的资源可以展开查看详情
                    if (resource.apply_status === 'completed') {
                      toggleResource(resource.id);
                    }
                  }}
                  style={{
                    cursor: resource.apply_status === 'completed' ? 'pointer' : 'default'
                  }}
                >
                  {resource.resource_address}
                  <button
                    className={styles.copyButton}
                    onClick={(e) => {
                      e.stopPropagation();
                      copyToClipboard(resource.resource_address, 'Resource address');
                    }}
                    title="Copy resource address"
                  >
                    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                      <rect x="9" y="9" width="13" height="13" rx="2" ry="2"></rect>
                      <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"></path>
                    </svg>
                  </button>
                  {/* 显示触发的 actions 指示器 */}
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
                <div 
                  className={getStatusTextAndClass(resource).className}
                  onClick={() => {
                    if (resource.apply_status === 'completed') {
                      toggleResource(resource.id);
                    }
                  }}
                  style={{
                    cursor: resource.apply_status === 'completed' ? 'pointer' : 'default'
                  }}
                >
                  {getStatusTextAndClass(resource).text}
                </div>
                {resource.apply_status === 'completed' && (
                  <div 
                    className={styles.expandIcon}
                    onClick={() => toggleResource(resource.id)}
                  >
                    {expandedResources.has(resource.id) ? '▼' : '▶'}
                  </div>
                )}
              </div>

              {/* 资源详情（仅completed状态时显示） */}
              {expandedResources.has(resource.id) && resource.apply_status === 'completed' && (
                <div className={styles.resourceDetails}>
                  {/* Resource ID - 如果有的话显示 */}
                  {resource.resource_id ? (
                    <div className={styles.detailItem}>
                      <span className={styles.detailLabel}>Resource ID:</span>
                      <span className={styles.detailValue}>
                        {resource.resource_id}
                        <button
                          className={styles.copyButton}
                          onClick={(e) => {
                            e.stopPropagation();
                            copyToClipboard(resource.resource_id!, 'Resource ID');
                          }}
                          title="Copy resource ID"
                        >
                          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                            <rect x="9" y="9" width="13" height="13" rx="2" ry="2"></rect>
                            <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"></path>
                          </svg>
                        </button>
                      </span>
                    </div>
                  ) : (
                    <div className={styles.detailItem}>
                      <span className={styles.detailLabel}>Resource ID:</span>
                      <span className={styles.detailValueMuted}>(not available)</span>
                    </div>
                  )}
                  
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
                                // 跳过空值和空数组，但如果已解析则显示实际值
                                if (value === null || (Array.isArray(value) && value.length === 0)) {
                                  // 检查是否在 config_unknown 中
                                  if (action.config_unknown?.[key]) {
                                    return (
                                      <div key={key} className={styles.actionConfigRow}>
                                        <span className={styles.actionConfigKey}>{key}:</span>
                                        <span className={isApplied ? styles.executedValue : styles.knownAfterApply}>
                                          {isApplied ? '(executed)' : '(known after apply)'}
                                        </span>
                                      </div>
                                    );
                                  }
                                  return null;
                                }
                                
                                // 检查这个值是否是从 state 解析出来的（原本在 config_unknown 中）
                                const wasUnknown = action.config_unknown?.[key];
                                const isResolved = action.config_resolved && wasUnknown;
                                
                                return (
                                  <div key={key} className={styles.actionConfigRow}>
                                    <span className={styles.actionConfigKey}>{key}:</span>
                                    <span className={isResolved ? styles.resolvedValue : styles.actionConfigValue}>
                                      {formatSimpleValue(value)}
                                      {isResolved && <span className={styles.resolvedBadge}>resolved</span>}
                                    </span>
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

      {/* Outputs Section */}
      {outputChanges.length > 0 && (
        <div className={styles.outputsSection}>
          <div 
            className={styles.outputsHeader}
            onClick={() => setOutputsExpanded(!outputsExpanded)}
          >
            <span className={styles.chevron}>{outputsExpanded ? '▼' : '▶'}</span>
            <span className={styles.outputsTitle}>Outputs</span>
            <span className={styles.outputsCount}>{outputChanges.length}</span>
          </div>
          {outputsExpanded && (
          <div className={styles.outputsList}>
            {outputChanges.map((output, index) => (
              <div key={index} className={styles.outputItem}>
                <div className={styles.outputName}>{output.name}</div>
                <div className={styles.outputValue}>
                  {output.sensitive ? (
                    <span className={styles.outputSensitive}>(sensitive value)</span>
                  ) : output.after_unknown ? (
                    <span className={styles.outputUnknown}>(known after apply)</span>
                  ) : (
                    <span className={styles.outputActualValue}>
                      {formatOutputValue(output.after)}
                      <button
                        className={styles.copyButton}
                        onClick={(e) => {
                          e.stopPropagation();
                          const value = typeof output.after === 'string' ? output.after : JSON.stringify(output.after);
                          copyToClipboard(value, 'Output value');
                        }}
                        title="Copy output value"
                      >
                        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                          <rect x="9" y="9" width="13" height="13" rx="2" ry="2"></rect>
                          <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"></path>
                        </svg>
                      </button>
                    </span>
                  )}
                </div>
              </div>
            ))}
          </div>
          )}
        </div>
      )}
    </div>
  );
};

export default ApplyingView;
