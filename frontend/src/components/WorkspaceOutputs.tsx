import React, { useState, useEffect, useCallback } from 'react';
import { useSearchParams, useNavigate } from 'react-router-dom';
import { Card, Button, Input, Select, Switch, message, Empty, Spin, Tag, Tooltip, Typography, Space, Popconfirm, Tabs, AutoComplete } from 'antd';
import { PlusOutlined, DeleteOutlined, SaveOutlined, ReloadOutlined, CopyOutlined, UndoOutlined, CloudDownloadOutlined } from '@ant-design/icons';
import styles from './WorkspaceOutputs.module.css';
import WorkspaceRemoteDataConfig from './WorkspaceRemoteDataConfig';

const { Text, Paragraph } = Typography;
const { Option } = Select;

// 静态输出的特殊资源名称标识
const STATIC_OUTPUT_RESOURCE_NAME = '__static__';

interface WorkspaceOutput {
  id: number;
  output_id: string;
  workspace_id: string;
  resource_name: string;
  output_name: string;
  output_value: string;
  description: string;
  sensitive: boolean;
  created_at: string;
  created_by: string;
}

// 判断是否为静态输出
const isStaticOutput = (output: WorkspaceOutput | EditableOutput): boolean => {
  // 如果是新建的行，通过 tempId 前缀判断
  const editableOutput = output as EditableOutput;
  if (editableOutput.isNew && editableOutput.tempId) {
    return editableOutput.tempId.startsWith('temp-static-');
  }
  // 已保存的行，通过 resource_name 判断
  return output.resource_name === STATIC_OUTPUT_RESOURCE_NAME;
};

interface EditableOutput extends WorkspaceOutput {
  isNew?: boolean;
  isModified?: boolean;
  isDeleted?: boolean;
  tempId?: string;
  // State 中的实际值
  stateValue?: any;
  hasStateValue?: boolean;
}

interface StateOutputInfo {
  check_results: any;
  lineage: string;
  outputs: Record<string, { value: any; type?: any; sensitive?: boolean }>;
  resources: any[];
  serial: number;
  terraform_version: string;
  version: number;
}

interface ResourceForOutput {
  resource_id: string;
  resource_name: string;
  resource_type: string;
  output_count: number;
}

// 可用的模块输出（用于智能提示）
interface AvailableOutput {
  name: string;
  alias?: string;
  type: string;
  description?: string;
  sensitive?: boolean;
  reference: string;
}

interface ResourceAvailableOutputs {
  resourceName: string;
  resourceId: string;
  resourceType: string;
  moduleName: string;
  moduleId?: number;
  outputs: AvailableOutput[];
}

interface WorkspaceOutputsProps {
  workspaceId: string;
}

const WorkspaceOutputs: React.FC<WorkspaceOutputsProps> = ({ workspaceId }) => {
  const [searchParams, setSearchParams] = useSearchParams();
  const navigate = useNavigate();
  
  // 从URL获取子标签，默认为outputs
  const subTabFromUrl = searchParams.get('subtab') || 'outputs';
  const [activeSubTab, setActiveSubTab] = useState(subTabFromUrl);
  
  const [outputs, setOutputs] = useState<EditableOutput[]>([]);
  const [originalOutputs, setOriginalOutputs] = useState<WorkspaceOutput[]>([]);
  const [stateOutputs, setStateOutputs] = useState<StateOutputInfo | null>(null);
  const [resources, setResources] = useState<ResourceForOutput[]>([]);
  const [availableOutputs, setAvailableOutputs] = useState<ResourceAvailableOutputs[]>([]);
  const [loading, setLoading] = useState(false);
  const [stateLoading, setStateLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [expandedOutputs, setExpandedOutputs] = useState<Set<string>>(new Set());
  
  // 同步URL参数到state
  useEffect(() => {
    const subtab = searchParams.get('subtab');
    if (subtab && subtab !== activeSubTab) {
      setActiveSubTab(subtab);
    }
  }, [searchParams]);
  
  // 处理子标签切换
  const handleSubTabChange = (key: string) => {
    setActiveSubTab(key);
    // 更新URL参数
    const newParams = new URLSearchParams(searchParams);
    newParams.set('subtab', key);
    setSearchParams(newParams, { replace: true });
  };

  const getAuthHeaders = () => {
    const token = localStorage.getItem('token');
    return {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${token}`,
    };
  };

  const fetchOutputs = useCallback(async () => {
    setLoading(true);
    try {
      const response = await fetch(`/api/v1/workspaces/${workspaceId}/outputs`, {
        headers: getAuthHeaders(),
      });
      if (response.ok) {
        const data = await response.json();
        if (data.code === 200) {
          const fetchedOutputs = data.outputs || [];
          setOriginalOutputs(fetchedOutputs);
          return fetchedOutputs;
        }
      }
      return [];
    } catch (error) {
      console.error('Failed to fetch outputs:', error);
      return [];
    } finally {
      setLoading(false);
    }
  }, [workspaceId]);

  const fetchStateOutputs = useCallback(async () => {
    setStateLoading(true);
    try {
      const response = await fetch(`/api/v1/workspaces/${workspaceId}/state-outputs`, {
        headers: getAuthHeaders(),
      });
      if (response.ok) {
        const data = await response.json();
        setStateOutputs(data);
        return data;
      }
      return null;
    } catch (error) {
      console.error('Failed to fetch state outputs:', error);
      return null;
    } finally {
      setStateLoading(false);
    }
  }, [workspaceId]);

  const fetchResources = useCallback(async () => {
    try {
      const response = await fetch(`/api/v1/workspaces/${workspaceId}/outputs/resources`, {
        headers: getAuthHeaders(),
      });
      if (response.ok) {
        const data = await response.json();
        if (data.code === 200) {
          setResources(data.resources || []);
        }
      }
    } catch (error) {
      console.error('Failed to fetch resources:', error);
    }
  }, [workspaceId]);

  // 获取可用的模块输出（用于智能提示）
  const fetchAvailableOutputs = useCallback(async () => {
    try {
      const response = await fetch(`/api/v1/workspaces/${workspaceId}/available-outputs`, {
        headers: getAuthHeaders(),
      });
      if (response.ok) {
        const data = await response.json();
        if (data.code === 200) {
          setAvailableOutputs(data.resources || []);
        }
      }
    } catch (error) {
      console.error('Failed to fetch available outputs:', error);
    }
  }, [workspaceId]);

  // 根据资源名称获取可用的输出列表
  const getAvailableOutputsForResource = useCallback((resourceName: string): AvailableOutput[] => {
    const resourceOutputs = availableOutputs.find(r => r.resourceName === resourceName);
    return resourceOutputs?.outputs || [];
  }, [availableOutputs]);

  // 合并配置的 outputs 和 state 中的值
  const mergeOutputsWithState = useCallback((configuredOutputs: WorkspaceOutput[], stateData: StateOutputInfo | null) => {
    const merged: EditableOutput[] = configuredOutputs.map(output => {
      // 构建 state 中的 key（格式：module_name-output_name）
      // output_value 格式：module.AWS_tesr-ccd_ken-aaa-2025-10-12-02.bucket_name
      const parts = output.output_value.split('.');
      const moduleName = parts[1] || '';
      const outputName = parts[2] || output.output_name;
      const stateKey = `${moduleName}-${outputName}`;
      
      const stateOutput = stateData?.outputs?.[stateKey];
      
      return {
        ...output,
        stateValue: stateOutput?.value,
        hasStateValue: stateOutput !== undefined,
      };
    });
    
    return merged;
  }, []);

  useEffect(() => {
    const loadData = async () => {
      const [configuredOutputs, stateData] = await Promise.all([
        fetchOutputs(),
        fetchStateOutputs(),
      ]);
      fetchResources();
      fetchAvailableOutputs();
      
      const merged = mergeOutputsWithState(configuredOutputs, stateData);
      setOutputs(merged);
    };
    
    loadData();
  }, [fetchOutputs, fetchStateOutputs, fetchResources, fetchAvailableOutputs, mergeOutputsWithState]);

  // 检查是否有未保存的更改
  const hasChanges = useCallback(() => {
    return outputs.some(o => o.isNew || o.isModified || o.isDeleted);
  }, [outputs]);

  // 切换展开/折叠
  const toggleExpand = (outputId: string) => {
    setExpandedOutputs(prev => {
      const newSet = new Set(prev);
      if (newSet.has(outputId)) {
        newSet.delete(outputId);
      } else {
        newSet.add(outputId);
      }
      return newSet;
    });
  };

  // 添加新行（资源关联输出）
  const handleAddRow = () => {
    const tempId = `temp-${Date.now()}`;
    const newOutput: EditableOutput = {
      id: 0,
      output_id: '',
      workspace_id: workspaceId,
      resource_name: '',
      output_name: '',
      output_value: '',
      description: '',
      sensitive: false,
      created_at: '',
      created_by: '',
      isNew: true,
      tempId,
    };
    setOutputs([...outputs, newOutput]);
    // 自动展开新添加的行
    setExpandedOutputs(prev => new Set(prev).add(tempId));
  };

  // 添加静态输出行
  const handleAddStaticRow = () => {
    const tempId = `temp-static-${Date.now()}`;
    const newOutput: EditableOutput = {
      id: 0,
      output_id: '',
      workspace_id: workspaceId,
      resource_name: STATIC_OUTPUT_RESOURCE_NAME,
      output_name: '',
      output_value: '',
      description: '',
      sensitive: false,
      created_at: '',
      created_by: '',
      isNew: true,
      tempId,
    };
    setOutputs([...outputs, newOutput]);
    // 自动展开新添加的行
    setExpandedOutputs(prev => new Set(prev).add(tempId));
  };

  // 更新行数据
  const handleUpdateRow = (index: number, field: keyof EditableOutput, value: any) => {
    const newOutputs = [...outputs];
    const output = newOutputs[index];
    
    // Sensitive 开关一旦打开就不能关闭
    if (field === 'sensitive' && output.sensitive && !value) {
      message.warning('Sensitive flag cannot be disabled once enabled');
      return;
    }
    
    (output as any)[field] = value;
    
    // 如果是资源名称变化，自动更新output_value
    if (field === 'resource_name' || field === 'output_name') {
      // 查找资源以获取正确的 resource_id
      const resource = resources.find(r => r.resource_name === output.resource_name);
      if (resource) {
        const moduleName = resource.resource_id.replace(/\./g, '_');
        output.output_value = `module.${moduleName}.${output.output_name}`;
      } else {
        output.output_value = `module.${output.resource_name}.${output.output_name}`;
      }
    }
    
    // 标记为已修改（如果不是新建的）
    if (!output.isNew) {
      output.isModified = true;
    }
    
    setOutputs(newOutputs);
  };

  // 标记删除
  const handleMarkDelete = (index: number) => {
    const newOutputs = [...outputs];
    const output = newOutputs[index];
    
    if (output.isNew) {
      // 新建的直接移除
      newOutputs.splice(index, 1);
    } else {
      // 已存在的标记为删除
      output.isDeleted = !output.isDeleted;
      output.isModified = false;
    }
    
    setOutputs(newOutputs);
  };

  // 撤销更改
  const handleRevert = () => {
    const merged = mergeOutputsWithState(originalOutputs, stateOutputs);
    setOutputs(merged);
  };

  // 刷新数据
  const handleRefresh = async () => {
    const [configuredOutputs, stateData] = await Promise.all([
      fetchOutputs(),
      fetchStateOutputs(),
    ]);
    
    const merged = mergeOutputsWithState(configuredOutputs, stateData);
    setOutputs(merged);
  };

  // 批量保存
  const handleSave = async () => {
    // 验证新建的行
    const newOutputs = outputs.filter(o => o.isNew && !o.isDeleted);
    for (const output of newOutputs) {
      const isStatic = isStaticOutput(output);
      if (isStatic) {
        // 静态输出：需要output_name和output_value
        if (!output.output_name || !output.output_value) {
          message.error('Please fill in output name and value for all static outputs');
          return;
        }
      } else {
        // 资源关联输出：需要resource_name和output_name
        if (!output.resource_name || !output.output_name) {
          message.error('Please fill in resource and output name for all resource outputs');
          return;
        }
      }
    }

    setSaving(true);
    try {
      // 分离资源关联输出和静态输出
      const createItems = outputs
        .filter(o => o.isNew && !o.isDeleted && !isStaticOutput(o))
        .map(o => ({
          resource_name: o.resource_name,
          output_name: o.output_name,
          description: o.description,
          sensitive: o.sensitive,
        }));

      const createStaticItems = outputs
        .filter(o => o.isNew && !o.isDeleted && isStaticOutput(o))
        .map(o => ({
          output_name: o.output_name,
          output_value: o.output_value,
          description: o.description,
          sensitive: o.sensitive,
        }));

      const updateItems = outputs
        .filter(o => o.isModified && !o.isNew && !o.isDeleted)
        .map(o => ({
          output_id: o.output_id,
          output_name: o.output_name,
          output_value: isStaticOutput(o) ? o.output_value : undefined, // 静态输出可以更新值
          description: o.description,
          sensitive: o.sensitive,
        }));

      const deleteIds = outputs
        .filter(o => o.isDeleted && !o.isNew)
        .map(o => o.output_id);

      const response = await fetch(`/api/v1/workspaces/${workspaceId}/outputs/batch`, {
        method: 'POST',
        headers: getAuthHeaders(),
        body: JSON.stringify({
          create: createItems,
          create_static: createStaticItems,
          update: updateItems,
          delete: deleteIds,
        }),
      });

      const data = await response.json();
      
      if (data.code === 200) {
        message.success(data.message || 'Changes saved successfully');
        
        if (data.errors && data.errors.length > 0) {
          data.errors.forEach((err: string) => message.warning(err));
        }
        
        // 刷新数据
        handleRefresh();
      } else {
        message.error(data.message || 'Failed to save changes');
      }
    } catch (error) {
      message.error('Failed to save changes');
    } finally {
      setSaving(false);
    }
  };

  const copyToClipboard = (text: string) => {
    navigator.clipboard.writeText(text);
    message.success('Copied to clipboard');
  };

  const formatOutputValue = (value: any): string => {
    if (value === null || value === undefined) return 'null';
    if (typeof value === 'object') return JSON.stringify(value, null, 2);
    return String(value);
  };

  // 获取行的状态样式
  const getRowClassName = (output: EditableOutput) => {
    if (output.isDeleted) return styles.deletedRow;
    if (output.isNew) return styles.newRow;
    if (output.isModified) return styles.modifiedRow;
    return '';
  };

  const isLoading = loading || stateLoading;

  // Outputs 内容渲染
  const renderOutputsContent = () => (
    <Card
      title={
        <Space>
          <span>Outputs</span>
          {hasChanges() && <Tag color="warning">Unsaved Changes</Tag>}
        </Space>
      }
      extra={
          <Space>
            <Button icon={<ReloadOutlined />} onClick={handleRefresh} loading={isLoading}>
              Refresh
            </Button>
            {hasChanges() && (
              <Popconfirm
                title="Discard all changes?"
                onConfirm={handleRevert}
                okText="Yes"
                cancelText="No"
              >
                <Button icon={<UndoOutlined />}>
                  Revert
                </Button>
              </Popconfirm>
            )}
            <Button icon={<PlusOutlined />} onClick={handleAddRow}>
              Add Resource Output
            </Button>
            <Button icon={<PlusOutlined />} onClick={handleAddStaticRow}>
              Add Static Output
            </Button>
            <Button
              type="primary"
              icon={<SaveOutlined />}
              onClick={handleSave}
              loading={saving}
              disabled={!hasChanges()}
            >
              Save Changes
            </Button>
          </Space>
        }
        className={styles.card}
      >
        {stateOutputs && (
          <div className={styles.stateInfo}>
            <Text type="secondary">
              Terraform {stateOutputs.terraform_version} | Serial: {stateOutputs.serial} | State Version: {stateOutputs.version}
            </Text>
          </div>
        )}

        {isLoading ? (
          <div className={styles.loading}>
            <Spin />
          </div>
        ) : outputs.length > 0 ? (
          <div className={styles.outputsList}>
            {outputs.map((output, index) => {
              const outputKey = output.output_id || output.tempId || `row-${index}`;
              const isExpanded = expandedOutputs.has(outputKey);
              
              return (
                <div 
                  key={outputKey} 
                  className={`${styles.outputCard} ${getRowClassName(output)}`}
                >
                  {/* 卡片头部 - 可点击展开 */}
                  <div 
                    className={styles.outputCardHeader}
                    onClick={() => toggleExpand(outputKey)}
                  >
                    <span className={styles.expandIcon}>{isExpanded ? '▼' : '▶'}</span>
                    
                    {output.isNew ? (
                      <Text type="secondary" italic>
                        {isStaticOutput(output) ? 'New Static Output' : 'New Resource Output'}
                      </Text>
                    ) : (
                      <Text strong className={styles.outputName}>
                        {isStaticOutput(output) 
                          ? `static-${output.output_name}` 
                          : `${output.resource_name}-${output.output_name}`}
                      </Text>
                    )}
                    
                    <div className={styles.outputTags}>
                      {isStaticOutput(output) && <Tag color="purple">Static</Tag>}
                      {!isStaticOutput(output) && <Tag color="blue">Resource</Tag>}
                      {output.sensitive && <Tag color="orange">Sensitive</Tag>}
                      {output.isDeleted && <Tag color="red">To Delete</Tag>}
                      {output.isNew && <Tag color="green">New</Tag>}
                      {output.isModified && <Tag color="blue">Modified</Tag>}
                      {output.hasStateValue && !output.isNew && <Tag color="cyan">Has Value</Tag>}
                    </div>
                    
                    <div className={styles.outputActions} onClick={(e) => e.stopPropagation()}>
                      {output.hasStateValue && !output.sensitive && (
                        <Tooltip title="Copy Value">
                          <Button
                            type="text"
                            size="small"
                            icon={<CopyOutlined />}
                            onClick={() => copyToClipboard(formatOutputValue(output.stateValue))}
                          />
                        </Tooltip>
                      )}
                      <Tooltip title={output.isDeleted ? 'Restore' : 'Delete'}>
                        <Button
                          type="text"
                          size="small"
                          danger={!output.isDeleted}
                          icon={output.isDeleted ? <UndoOutlined /> : <DeleteOutlined />}
                          onClick={() => handleMarkDelete(index)}
                        />
                      </Tooltip>
                    </div>
                  </div>
                  
                  {/* 展开的内容 */}
                  {isExpanded && (
                    <div className={styles.outputCardBody}>
                      {/* 配置区域 - 根据输出类型显示不同的表单 */}
                      <div className={styles.configSection}>
                        {isStaticOutput(output) ? (
                          // 静态输出表单
                          <>
                            <div className={styles.configRow}>
                              <label>Type:</label>
                              <Tag color="purple">Static Value</Tag>
                            </div>
                            
                            <div className={styles.configRow}>
                              <label>Output Name:</label>
                              <Input
                                value={output.output_name}
                                placeholder="e.g., my_custom_output"
                                onChange={(e) => handleUpdateRow(index, 'output_name', e.target.value)}
                                disabled={output.isDeleted || !output.isNew}
                                style={{ flex: 1 }}
                              />
                            </div>
                            
                            <div className={styles.configRow}>
                              <label>Value:</label>
                              <Input.TextArea
                                value={output.output_value}
                                placeholder='e.g., "my-static-value" or local.computed_value'
                                onChange={(e) => handleUpdateRow(index, 'output_value', e.target.value)}
                                disabled={output.isDeleted}
                                style={{ flex: 1 }}
                                rows={2}
                              />
                            </div>
                            
                            <div className={styles.configRow}>
                              <label>Description:</label>
                              <Input
                                value={output.description}
                                placeholder="Optional description"
                                onChange={(e) => handleUpdateRow(index, 'description', e.target.value)}
                                disabled={output.isDeleted}
                                style={{ flex: 1 }}
                              />
                            </div>
                            
                            <div className={styles.configRow}>
                              <label>Sensitive:</label>
                              <Tooltip title={output.sensitive ? "Sensitive flag cannot be disabled once enabled" : ""}>
                                <Switch
                                  checked={output.sensitive}
                                  onChange={(checked) => handleUpdateRow(index, 'sensitive', checked)}
                                  disabled={output.isDeleted || output.sensitive}
                                  size="small"
                                />
                              </Tooltip>
                            </div>
                          </>
                        ) : (
                          // 资源关联输出表单
                          <>
                            <div className={styles.configRow}>
                              <label>Type:</label>
                              <Tag color="blue">Resource Output</Tag>
                            </div>
                            
                            <div className={styles.configRow}>
                              <label>Resource:</label>
                              {output.isNew ? (
                                <Select
                                  value={output.resource_name || undefined}
                                  placeholder="Select resource"
                                  style={{ flex: 1 }}
                                  onChange={(value) => handleUpdateRow(index, 'resource_name', value)}
                                  showSearch
                                  optionFilterProp="children"
                                  disabled={output.isDeleted}
                                >
                                  {resources.map(resource => (
                                    <Option key={resource.resource_id} value={resource.resource_name}>
                                      {resource.resource_name}
                                    </Option>
                                  ))}
                                </Select>
                              ) : (
                                <Tag color="blue">{output.resource_name}</Tag>
                              )}
                            </div>
                            
                            <div className={styles.configRow}>
                              <label>Output Name:</label>
                              {(() => {
                                const availableOuts = getAvailableOutputsForResource(output.resource_name);
                                if (availableOuts.length > 0) {
                                  // 有可用的模块输出，显示带智能提示的 AutoComplete
                                  return (
                                    <AutoComplete
                                      value={output.output_name}
                                      placeholder="Select or type output name"
                                      style={{ flex: 1 }}
                                      onChange={(value) => handleUpdateRow(index, 'output_name', value)}
                                      disabled={output.isDeleted}
                                      options={availableOuts.map(o => ({
                                        value: o.name,
                                        label: (
                                          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                                            <span>
                                              <strong>{o.name}</strong>
                                              {o.alias && <Text type="secondary" style={{ marginLeft: 8 }}>({o.alias})</Text>}
                                            </span>
                                            <span>
                                              <Tag color="default" style={{ marginLeft: 8 }}>{o.type}</Tag>
                                              {o.sensitive && <Tag color="orange">Sensitive</Tag>}
                                            </span>
                                          </div>
                                        ),
                                      }))}
                                      filterOption={(inputValue, option) =>
                                        option?.value.toLowerCase().includes(inputValue.toLowerCase()) || false
                                      }
                                    />
                                  );
                                } else {
                                  // 没有可用的模块输出，显示普通输入框
                                  return (
                                    <Input
                                      value={output.output_name}
                                      placeholder="e.g., bucket_arn"
                                      onChange={(e) => handleUpdateRow(index, 'output_name', e.target.value)}
                                      disabled={output.isDeleted}
                                      style={{ flex: 1 }}
                                    />
                                  );
                                }
                              })()}
                            </div>
                            
                            <div className={styles.configRow}>
                              <label>Value Expression:</label>
                              <Text code style={{ flex: 1, padding: '4px 8px', background: '#f5f5f5', borderRadius: '4px' }}>
                                {output.output_value || '(auto-generated after selecting resource and output name)'}
                              </Text>
                            </div>
                            
                            <div className={styles.configRow}>
                              <label>Description:</label>
                              <Input
                                value={output.description}
                                placeholder="Optional description"
                                onChange={(e) => handleUpdateRow(index, 'description', e.target.value)}
                                disabled={output.isDeleted}
                                style={{ flex: 1 }}
                              />
                            </div>
                            
                            <div className={styles.configRow}>
                              <label>Sensitive:</label>
                              <Tooltip title={output.sensitive ? "Sensitive flag cannot be disabled once enabled" : ""}>
                                <Switch
                                  checked={output.sensitive}
                                  onChange={(checked) => handleUpdateRow(index, 'sensitive', checked)}
                                  disabled={output.isDeleted || output.sensitive}
                                  size="small"
                                />
                              </Tooltip>
                            </div>
                          </>
                        )}
                      </div>
                      
                      {/* 值区域 */}
                      {output.hasStateValue && (
                        <div className={styles.valueSection}>
                          <label>Current Value:</label>
                          {output.sensitive ? (
                            <Text type="secondary" italic>***SENSITIVE*** (Use API to retrieve)</Text>
                          ) : (
                            <Paragraph
                              code
                              copyable={false}
                              className={styles.valueDisplay}
                            >
                              {formatOutputValue(output.stateValue)}
                            </Paragraph>
                          )}
                        </div>
                      )}
                      
                      {!output.hasStateValue && !output.isNew && (
                        <div className={styles.valueSection}>
                          <Text type="secondary" italic>
                            No value in state yet. Run a plan/apply to generate the output value.
                          </Text>
                        </div>
                      )}
                    </div>
                  )}
                </div>
              );
            })}
          </div>
        ) : (
          <Empty
            description="No outputs configured"
            image={Empty.PRESENTED_IMAGE_SIMPLE}
          >
            <Button type="primary" onClick={handleAddRow}>
              Add Your First Output
            </Button>
          </Empty>
        )}
    </Card>
  );

  // Tab items
  const tabItems = [
    {
      key: 'outputs',
      label: (
        <span>
          <SaveOutlined />
          Outputs
        </span>
      ),
      children: renderOutputsContent(),
    },
    {
      key: 'remote-data',
      label: (
        <span>
          <CloudDownloadOutlined />
          Remote Data
        </span>
      ),
      children: <WorkspaceRemoteDataConfig workspaceId={workspaceId} />,
    },
  ];

  return (
    <div className={styles.container}>
      <Tabs
        activeKey={activeSubTab}
        onChange={handleSubTabChange}
        items={tabItems}
        className={styles.tabs}
      />
    </div>
  );
};

export default WorkspaceOutputs;
