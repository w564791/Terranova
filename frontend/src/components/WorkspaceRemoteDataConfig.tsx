import React, { useState, useEffect, useCallback } from 'react';
import { Card, Button, Select, Input, Empty, Spin, Tag, Typography, Space, Divider, Alert, Tooltip } from 'antd';
import { PlusOutlined, DeleteOutlined, ReloadOutlined, LinkOutlined, LockOutlined, UnlockOutlined, CopyOutlined, DownOutlined, RightOutlined, CloseOutlined, SaveOutlined, EditOutlined, WarningOutlined, CheckCircleOutlined } from '@ant-design/icons';
import ConfirmDialog from './ConfirmDialog';
import { useToast } from '../contexts/ToastContext';
import styles from './WorkspaceRemoteDataConfig.module.css';

const { Text, Paragraph } = Typography;
const { Option } = Select;
const { TextArea } = Input;


interface RemoteDataItem {
  remote_data_id: string;
  workspace_id: string;
  source_workspace_id: string;
  source_workspace_name?: string;
  data_name: string;
  description: string;
  available_outputs?: OutputKeyInfo[];
}

interface AccessibleWorkspace {
  workspace_id: string;
  name: string;
}

interface OutputKeyInfo {
  key: string;
  type?: string;
  sensitive: boolean;
  value?: any;
  status?: 'available' | 'pending';  // available = 已apply有值, pending = 已配置但未apply
  description?: string;
}

interface AllowedWorkspace {
  workspace_id: string;
  name: string;
}

interface WorkspaceRemoteDataConfigProps {
  workspaceId: string;
}

const WorkspaceRemoteDataConfig: React.FC<WorkspaceRemoteDataConfigProps> = ({ workspaceId }) => {
  const { success: showSuccess, error: showError, info: showInfo } = useToast();
  const [remoteDataList, setRemoteDataList] = useState<RemoteDataItem[]>([]);
  const [accessibleWorkspaces, setAccessibleWorkspaces] = useState<AccessibleWorkspace[]>([]);
  const [loading, setLoading] = useState(false);
  const [expandedOutputs, setExpandedOutputs] = useState<Set<string>>(new Set());
  
  // 内联表单展开状态
  const [showAddForm, setShowAddForm] = useState(false);
  const [showSharingForm, setShowSharingForm] = useState(false);
  
  // Sharing settings
  const [sharingMode, setSharingMode] = useState<string>('none');
  const [editingSharingMode, setEditingSharingMode] = useState<string>('none');
  const [allowedWorkspaces, setAllowedWorkspaces] = useState<AllowedWorkspace[]>([]);
  const [editingAllowedWorkspaces, setEditingAllowedWorkspaces] = useState<AllowedWorkspace[]>([]);
  const [allWorkspaces, setAllWorkspaces] = useState<AccessibleWorkspace[]>([]);
  const [savingSharing, setSavingSharing] = useState(false);
  
  // Add form
  const [selectedSourceWorkspace, setSelectedSourceWorkspace] = useState<string>('');
  const [sourceOutputs, setSourceOutputs] = useState<OutputKeyInfo[]>([]);
  const [loadingOutputs, setLoadingOutputs] = useState(false);
  const [dataName, setDataName] = useState('');
  const [description, setDescription] = useState('');
  const [adding, setAdding] = useState(false);
  
  // Delete confirmation
  const [deleteConfirm, setDeleteConfirm] = useState<{ show: boolean; id: string; name: string }>({
    show: false, id: '', name: ''
  });
  const [deleting, setDeleting] = useState(false);
  
  // Edit form
  const [editingItem, setEditingItem] = useState<RemoteDataItem | null>(null);
  const [editDataName, setEditDataName] = useState('');
  const [editDescription, setEditDescription] = useState('');
  const [saving, setSaving] = useState(false);

  const getAuthHeaders = () => {
    const token = localStorage.getItem('token');
    return {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${token}`,
    };
  };

  const fetchRemoteData = useCallback(async () => {
    setLoading(true);
    try {
      const response = await fetch(`/api/v1/workspaces/${workspaceId}/remote-data`, {
        headers: getAuthHeaders(),
      });
      if (response.ok) {
        const data = await response.json();
        if (data.code === 200) {
          const remoteData = data.remote_data || [];
          
          const remoteDataWithOutputs = await Promise.all(
            remoteData.map(async (item: RemoteDataItem) => {
              try {
                const outputsResponse = await fetch(
                  `/api/v1/workspaces/${workspaceId}/remote-data/source-outputs?source_workspace_id=${item.source_workspace_id}`,
                  { headers: getAuthHeaders() }
                );
                if (outputsResponse.ok) {
                  const outputsData = await outputsResponse.json();
                  if (outputsData.code === 200) {
                    return { ...item, available_outputs: outputsData.outputs || [] };
                  }
                }
              } catch (error) {
                console.error(`Failed to fetch outputs for ${item.source_workspace_id}:`, error);
              }
              return item;
            })
          );
          
          setRemoteDataList(remoteDataWithOutputs);
        }
      }
    } catch (error) {
      console.error('Failed to fetch remote data:', error);
    } finally {
      setLoading(false);
    }
  }, [workspaceId]);

  const fetchAccessibleWorkspaces = useCallback(async () => {
    try {
      const response = await fetch(`/api/v1/workspaces/${workspaceId}/remote-data/accessible-workspaces`, {
        headers: getAuthHeaders(),
      });
      if (response.ok) {
        const data = await response.json();
        if (data.code === 200) {
          setAccessibleWorkspaces(data.workspaces || []);
        }
      }
    } catch (error) {
      console.error('Failed to fetch accessible workspaces:', error);
    }
  }, [workspaceId]);

  const fetchSharingSettings = useCallback(async () => {
    try {
      const response = await fetch(`/api/v1/workspaces/${workspaceId}/outputs-sharing`, {
        headers: getAuthHeaders(),
      });
      if (response.ok) {
        const data = await response.json();
        if (data.code === 200) {
          setSharingMode(data.sharing_mode || 'none');
          setAllowedWorkspaces(data.allowed_workspaces || []);
        }
      }
    } catch (error) {
      console.error('Failed to fetch sharing settings:', error);
    }
  }, [workspaceId]);

  const fetchAllWorkspaces = useCallback(async () => {
    try {
      const response = await fetch(`/api/v1/workspaces`, {
        headers: getAuthHeaders(),
      });
      if (response.ok) {
        const data = await response.json();
        if (data.code === 200) {
          const workspaceList = data.data?.items || data.workspaces || [];
          const filtered = workspaceList.filter(
            (ws: any) => ws.workspace_id !== workspaceId
          );
          setAllWorkspaces(filtered.map((ws: any) => ({
            workspace_id: ws.workspace_id,
            name: ws.name,
          })));
        }
      }
    } catch (error) {
      console.error('Failed to fetch all workspaces:', error);
    }
  }, [workspaceId]);

  useEffect(() => {
    fetchRemoteData();
    fetchAccessibleWorkspaces();
    fetchSharingSettings();
    fetchAllWorkspaces();
  }, [fetchRemoteData, fetchAccessibleWorkspaces, fetchSharingSettings, fetchAllWorkspaces]);

  const fetchSourceOutputs = async (sourceWorkspaceId: string) => {
    setLoadingOutputs(true);
    try {
      const response = await fetch(
        `/api/v1/workspaces/${workspaceId}/remote-data/source-outputs?source_workspace_id=${sourceWorkspaceId}`,
        { headers: getAuthHeaders() }
      );
      if (response.ok) {
        const data = await response.json();
        if (data.code === 200) {
          setSourceOutputs(data.outputs || []);
        }
      }
    } catch (error) {
      console.error('Failed to fetch source outputs:', error);
    } finally {
      setLoadingOutputs(false);
    }
  };

  const handleSourceWorkspaceChange = (value: string) => {
    setSelectedSourceWorkspace(value);
    if (value) {
      fetchSourceOutputs(value);
    } else {
      setSourceOutputs([]);
    }
  };

  // 过滤掉已经添加过的 workspace
  const availableWorkspacesForAdd = accessibleWorkspaces.filter(
    ws => !remoteDataList.some(rd => rd.source_workspace_id === ws.workspace_id)
  );

  const handleAddRemoteData = async () => {
    if (!selectedSourceWorkspace || !dataName) {
      showError('Please select source workspace and enter data name');
      return;
    }

    setAdding(true);
    try {
      // 使用完整的 data_name（带 workspace 前缀）
      const fullDataName = getSelectedSourceWorkspaceName() + '-' + dataName;
      const response = await fetch(`/api/v1/workspaces/${workspaceId}/remote-data`, {
        method: 'POST',
        headers: getAuthHeaders(),
        body: JSON.stringify({
          source_workspace_id: selectedSourceWorkspace,
          data_name: fullDataName,
          description: description,
        }),
      });

      const data = await response.json();
      if (data.code === 201) {
        showSuccess('Remote data reference added successfully');
        setShowAddForm(false);
        resetAddForm();
        fetchRemoteData();
      } else {
        showError(data.message || 'Failed to add remote data reference');
      }
    } catch (error) {
      showError('Failed to add remote data reference');
    } finally {
      setAdding(false);
    }
  };

  const handleDeleteClick = (remoteDataId: string, dataName: string) => {
    setDeleteConfirm({ show: true, id: remoteDataId, name: dataName });
  };

  const confirmDelete = async () => {
    const { id } = deleteConfirm;
    setDeleting(true);
    try {
      const response = await fetch(`/api/v1/workspaces/${workspaceId}/remote-data/${id}`, {
        method: 'DELETE',
        headers: getAuthHeaders(),
      });

      const data = await response.json();
      if (data.code === 200) {
        showSuccess('Remote data reference deleted successfully');
        fetchRemoteData();
      } else {
        showError(data.message || 'Failed to delete remote data reference');
      }
    } catch (error) {
      showError('Failed to delete remote data reference');
    } finally {
      setDeleting(false);
      setDeleteConfirm({ show: false, id: '', name: '' });
    }
  };

  // 获取选中的源 workspace 名称
  const getSelectedSourceWorkspaceName = () => {
    const ws = accessibleWorkspaces.find(w => w.workspace_id === selectedSourceWorkspace);
    return ws?.name || '';
  };

  const handleEditClick = (item: RemoteDataItem) => {
    setEditingItem(item);
    // 从 data_name 中提取用户输入的部分（去掉 workspace 前缀）
    const prefix = (item.source_workspace_name || '') + '-';
    const userPart = item.data_name.startsWith(prefix) 
      ? item.data_name.substring(prefix.length) 
      : item.data_name;
    setEditDataName(userPart);
    setEditDescription(item.description || '');
  };

  const handleCancelEdit = () => {
    setEditingItem(null);
    setEditDataName('');
    setEditDescription('');
  };

  const handleSaveEdit = async () => {
    if (!editingItem || !editDataName) {
      showError('Data name is required');
      return;
    }

    setSaving(true);
    try {
      const fullDataName = (editingItem.source_workspace_name || '') + '-' + editDataName;
      const response = await fetch(`/api/v1/workspaces/${workspaceId}/remote-data/${editingItem.remote_data_id}`, {
        method: 'PUT',
        headers: getAuthHeaders(),
        body: JSON.stringify({
          data_name: fullDataName,
          description: editDescription,
        }),
      });

      const data = await response.json();
      if (data.code === 200) {
        showSuccess('Remote data reference updated successfully');
        handleCancelEdit();
        fetchRemoteData();
      } else {
        showError(data.message || 'Failed to update remote data reference');
      }
    } catch (error) {
      showError('Failed to update remote data reference');
    } finally {
      setSaving(false);
    }
  };

  const handleUpdateSharing = async () => {
    setSavingSharing(true);
    try {
      const response = await fetch(`/api/v1/workspaces/${workspaceId}/outputs-sharing`, {
        method: 'PUT',
        headers: getAuthHeaders(),
        body: JSON.stringify({
          sharing_mode: editingSharingMode,
          allowed_workspace_ids: editingAllowedWorkspaces.map(ws => ws.workspace_id),
        }),
      });

      const data = await response.json();
      if (data.code === 200) {
        showSuccess('Sharing settings updated successfully');
        setSharingMode(editingSharingMode);
        setAllowedWorkspaces(editingAllowedWorkspaces);
        setShowSharingForm(false);
      } else {
        showError(data.message || 'Failed to update sharing settings');
      }
    } catch (error) {
      showError('Failed to update sharing settings');
    } finally {
      setSavingSharing(false);
    }
  };

  const resetAddForm = () => {
    setSelectedSourceWorkspace('');
    setSourceOutputs([]);
    setDataName('');
    setDescription('');
  };

  const openSharingForm = () => {
    setEditingSharingMode(sharingMode);
    setEditingAllowedWorkspaces([...allowedWorkspaces]);
    setShowSharingForm(true);
  };

  const getSharingModeIcon = () => {
    switch (sharingMode) {
      case 'all':
        return <UnlockOutlined style={{ color: '#52c41a' }} />;
      case 'specific':
        return <LinkOutlined style={{ color: '#1890ff' }} />;
      default:
        return <LockOutlined style={{ color: '#ff4d4f' }} />;
    }
  };

  const getSharingModeText = () => {
    switch (sharingMode) {
      case 'all':
        return 'Shared with all workspaces';
      case 'specific':
        return `Shared with ${allowedWorkspaces.length} workspace(s)`;
      default:
        return 'Not shared';
    }
  };

  return (
    <div className={styles.container}>
      {/* Outputs Sharing Settings */}
      <Card
        title="Outputs Sharing"
        extra={
          !showSharingForm && (
            <Button icon={<EditOutlined />} onClick={openSharingForm}>
              Configure
            </Button>
          )
        }
        className={styles.card}
        size="small"
      >
        {showSharingForm ? (
          <div className={styles.inlineForm}>
            <div className={styles.formItem}>
              <label className={styles.formLabel}>Sharing Mode</label>
              <Select
                value={editingSharingMode}
                onChange={(value) => {
                  setEditingSharingMode(value);
                  if (value !== 'specific') {
                    setEditingAllowedWorkspaces([]);
                  }
                }}
                style={{ width: '100%' }}
              >
                <Option value="none">
                  <Space>
                    <LockOutlined />
                    <span>None - Do not share outputs</span>
                  </Space>
                </Option>
                <Option value="all">
                  <Space>
                    <UnlockOutlined />
                    <span>All - Share with all workspaces</span>
                  </Space>
                </Option>
                <Option value="specific">
                  <Space>
                    <LinkOutlined />
                    <span>Specific - Share with selected workspaces</span>
                  </Space>
                </Option>
              </Select>
            </div>

            {editingSharingMode === 'specific' && (
              <div className={styles.formItem}>
                <label className={styles.formLabel}>Allowed Workspaces</label>
                <Select
                  mode="multiple"
                  placeholder="Select workspaces to share with"
                  value={editingAllowedWorkspaces.map(ws => ws.workspace_id)}
                  onChange={(values) => {
                    const selected = allWorkspaces.filter(ws => values.includes(ws.workspace_id));
                    setEditingAllowedWorkspaces(selected);
                  }}
                  style={{ width: '100%' }}
                  optionFilterProp="children"
                >
                  {allWorkspaces.map(ws => (
                    <Option key={ws.workspace_id} value={ws.workspace_id}>
                      {ws.name}
                    </Option>
                  ))}
                </Select>
              </div>
            )}

            <div className={styles.formActions}>
              <Button 
                onClick={() => setShowSharingForm(false)}
                icon={<CloseOutlined />}
              >
                Cancel
              </Button>
              <Button 
                type="primary" 
                onClick={handleUpdateSharing}
                loading={savingSharing}
                icon={<SaveOutlined />}
              >
                Save
              </Button>
            </div>
          </div>
        ) : (
          <>
            <div className={styles.sharingStatus}>
              {getSharingModeIcon()}
              <Text style={{ marginLeft: 8 }}>{getSharingModeText()}</Text>
            </div>
            <Paragraph type="secondary" style={{ marginTop: 8, marginBottom: 0 }}>
              Control which workspaces can reference this workspace's outputs as remote data.
            </Paragraph>
          </>
        )}
      </Card>

      {/* Add Remote Data Form (inline) */}
      {showAddForm && (
        <Card
          title="Add Remote Data Reference"
          extra={
            <Button 
              type="text" 
              icon={<CloseOutlined />} 
              onClick={() => {
                setShowAddForm(false);
                resetAddForm();
              }}
            />
          }
          className={styles.card}
        >
          <div className={styles.inlineForm}>
            <div className={styles.formItem}>
              <label className={styles.formLabel}>Source Workspace *</label>
              <Select
                placeholder="Select a workspace to reference"
                value={selectedSourceWorkspace || undefined}
                onChange={handleSourceWorkspaceChange}
                style={{ width: '100%' }}
                showSearch
                optionFilterProp="children"
              >
                {availableWorkspacesForAdd.map(ws => (
                  <Option key={ws.workspace_id} value={ws.workspace_id}>
                    {ws.name} ({ws.workspace_id})
                  </Option>
                ))}
              </Select>
              {availableWorkspacesForAdd.length === 0 && accessibleWorkspaces.length > 0 && (
                <Alert
                  message="All accessible workspaces have been added"
                  description="You have already added references to all available workspaces."
                  type="info"
                  showIcon
                  style={{ marginTop: 8 }}
                />
              )}
              {accessibleWorkspaces.length === 0 && (
                <Alert
                  message="No accessible workspaces"
                  description="No workspaces are sharing their outputs with you. Ask workspace owners to enable sharing."
                  type="info"
                  showIcon
                  style={{ marginTop: 8 }}
                />
              )}
            </div>

            {selectedSourceWorkspace && (
              <>
                <div className={styles.formItem}>
                  <label className={styles.formLabel}>Available Outputs</label>
                  {loadingOutputs ? (
                    <Spin size="small" />
                  ) : sourceOutputs.length > 0 ? (
                    <div className={styles.outputsList}>
                      {sourceOutputs.map(output => (
                        <Tooltip 
                          key={output.key}
                          title={output.status === 'pending' 
                            ? 'This output has not been applied yet. Click to copy output key.' 
                            : `Click to copy: ${output.key}`}
                        >
                          <Tag
                            color={output.status === 'pending' ? 'warning' : (output.sensitive ? 'orange' : 'default')}
                            icon={output.status === 'pending' ? <WarningOutlined /> : (output.status === 'available' ? <CheckCircleOutlined /> : undefined)}
                            style={{ marginBottom: 4, cursor: 'pointer' }}
                            onClick={() => {
                              navigator.clipboard.writeText(output.key);
                              showInfo(`Copied: ${output.key}`);
                            }}
                          >
                            {output.key}
                            {output.sensitive && ' (sensitive)'}
                            {output.status === 'pending' && ' (pending)'}
                          </Tag>
                        </Tooltip>
                      ))}
                      {sourceOutputs.some(o => o.status === 'pending') && (
                        <Alert
                          message="Some outputs are pending"
                          description="The source workspace has configured outputs that have not been applied yet. You can still reference them, but they won't have values until the source workspace runs apply."
                          type="warning"
                          showIcon
                          style={{ marginTop: 8 }}
                        />
                      )}
                    </div>
                  ) : (
                    <Text type="secondary">No outputs available in this workspace</Text>
                  )}
                </div>

                <Divider style={{ margin: '12px 0' }} />

                <div className={styles.formItem}>
                  <label className={styles.formLabel}>Data Name *</label>
                  <Input
                    addonBefore={<span style={{ color: '#1890ff' }}>{getSelectedSourceWorkspaceName()}-</span>}
                    placeholder="e.g., outputs, data"
                    value={dataName}
                    onChange={(e) => setDataName(e.target.value)}
                  />
                  <Text type="secondary" style={{ fontSize: 12 }}>
                    Full name: <code>{getSelectedSourceWorkspaceName()}-{dataName || 'your_name'}</code>
                  </Text>
                </div>

                <div className={styles.formItem}>
                  <label className={styles.formLabel}>Description</label>
                  <TextArea
                    placeholder="Optional description"
                    value={description}
                    onChange={(e) => setDescription(e.target.value)}
                    rows={2}
                  />
                </div>
              </>
            )}

            <div className={styles.formActions}>
              <Button 
                onClick={() => {
                  setShowAddForm(false);
                  resetAddForm();
                }}
                icon={<CloseOutlined />}
              >
                Cancel
              </Button>
              <Button 
                type="primary" 
                onClick={handleAddRemoteData}
                loading={adding}
                disabled={!selectedSourceWorkspace || !dataName}
                icon={<SaveOutlined />}
              >
                Add Reference
              </Button>
            </div>
          </div>
        </Card>
      )}

      {/* Remote Data References */}
      <Card
        title="Remote Data References"
        extra={
          <Space>
            <Button icon={<ReloadOutlined />} onClick={fetchRemoteData} loading={loading}>
              Refresh
            </Button>
            {!showAddForm && (
              <Button type="primary" icon={<PlusOutlined />} onClick={() => setShowAddForm(true)}>
                Add Reference
              </Button>
            )}
          </Space>
        }
        className={styles.card}
      >
        {loading ? (
          <div className={styles.loading}>
            <Spin />
          </div>
        ) : remoteDataList.length > 0 ? (
          <div className={styles.remoteDataList}>
            {remoteDataList.map((item) => (
              <div key={item.remote_data_id} className={styles.remoteDataItem}>
                {editingItem?.remote_data_id === item.remote_data_id ? (
                  // 编辑模式
                  <div className={styles.inlineForm}>
                    <div className={styles.formItem}>
                      <label className={styles.formLabel}>Data Name *</label>
                      <Input
                        addonBefore={<span style={{ color: '#1890ff' }}>{item.source_workspace_name || ''}-</span>}
                        placeholder="e.g., outputs, data"
                        value={editDataName}
                        onChange={(e) => setEditDataName(e.target.value)}
                      />
                    </div>
                    <div className={styles.formItem}>
                      <label className={styles.formLabel}>Description</label>
                      <TextArea
                        placeholder="Optional description"
                        value={editDescription}
                        onChange={(e) => setEditDescription(e.target.value)}
                        rows={2}
                      />
                    </div>
                    <div className={styles.formActions}>
                      <Button onClick={handleCancelEdit} icon={<CloseOutlined />}>
                        Cancel
                      </Button>
                      <Button 
                        type="primary" 
                        onClick={handleSaveEdit}
                        loading={saving}
                        disabled={!editDataName}
                        icon={<SaveOutlined />}
                      >
                        Save
                      </Button>
                    </div>
                  </div>
                ) : (
                  // 显示模式
                  <>
                    <div className={styles.itemHeader}>
                      <Text strong>{item.data_name}</Text>
                      <Space>
                        <Button 
                          type="text" 
                          size="small" 
                          icon={<EditOutlined />} 
                          onClick={() => handleEditClick(item)}
                        />
                        <Button 
                          type="text" 
                          danger 
                          size="small" 
                          icon={<DeleteOutlined />} 
                          onClick={() => handleDeleteClick(item.remote_data_id, item.data_name)}
                        />
                      </Space>
                    </div>
                    <div className={styles.itemContent}>
                  <div className={styles.itemRow}>
                    <Text type="secondary">Source Workspace:</Text>
                    <Tag color="blue">{item.source_workspace_name || item.source_workspace_id}</Tag>
                  </div>
                  {item.description && (
                    <div className={styles.itemRow}>
                      <Text type="secondary">Description:</Text>
                      <Text>{item.description}</Text>
                    </div>
                  )}
                  <div className={styles.usageSection}>
                    <Text type="secondary" style={{ fontSize: 12 }}>Available Outputs:</Text>
                    {item.available_outputs && item.available_outputs.length > 0 ? (
                      <div className={styles.outputsUsageList}>
                        {item.available_outputs.map(output => {
                          const code = `\${local.${item.data_name}.${output.key}.value}`;
                          const outputKey = `${item.remote_data_id}-${output.key}`;
                          const isExpanded = expandedOutputs.has(outputKey);
                          const hasValue = !output.sensitive && output.value !== undefined;
                          const isPending = output.status === 'pending';
                          
                          const formatValue = (val: any): string => {
                            if (val === null || val === undefined) return 'null';
                            if (typeof val === 'object') return JSON.stringify(val, null, 2);
                            return String(val);
                          };
                          
                          return (
                            <div key={output.key} className={styles.outputUsageWrapper}>
                              <div 
                                className={`${styles.outputUsageItem} ${hasValue ? styles.clickable : ''}`}
                                onClick={() => {
                                  if (hasValue) {
                                    setExpandedOutputs(prev => {
                                      const newSet = new Set(prev);
                                      if (newSet.has(outputKey)) {
                                        newSet.delete(outputKey);
                                      } else {
                                        newSet.add(outputKey);
                                      }
                                      return newSet;
                                    });
                                  }
                                }}
                              >
                                {hasValue && (
                                  <span className={styles.expandIcon}>
                                    {isExpanded ? <DownOutlined /> : <RightOutlined />}
                                  </span>
                                )}
                                <code className={styles.outputCode}>
                                  {code}
                                </code>
                                {output.sensitive && (
                                  <Tag color="orange" style={{ marginLeft: 8, fontSize: 10 }}>sensitive</Tag>
                                )}
                                {isPending && (
                                  <Tooltip title="This output has not been applied yet. The source workspace needs to run apply first.">
                                    <Tag color="warning" icon={<WarningOutlined />} style={{ marginLeft: 8, fontSize: 10 }}>pending</Tag>
                                  </Tooltip>
                                )}
                                <Button
                                  type="text"
                                  size="small"
                                  icon={<CopyOutlined />}
                                  className={styles.copyButton}
                                  onClick={(e) => {
                                    e.stopPropagation();
                                    navigator.clipboard.writeText(code);
                                    showInfo('Copied!');
                                  }}
                                />
                              </div>
                              {isExpanded && hasValue && (
                                <div className={styles.outputValueContainer}>
                                  <pre className={styles.outputValue}>
                                    {formatValue(output.value)}
                                  </pre>
                                </div>
                              )}
                            </div>
                          );
                        })}
                        {item.available_outputs.some(o => o.status === 'pending') && (
                          <Alert
                            message="Some outputs are pending apply"
                            description="The source workspace needs to run apply for these outputs to have values."
                            type="warning"
                            showIcon
                            style={{ marginTop: 8 }}
                          />
                        )}
                      </div>
                    ) : (
                      <Text type="secondary" style={{ fontSize: 12, display: 'block', marginTop: 4 }}>
                        No outputs available in source workspace
                      </Text>
                    )}
                  </div>
                    </div>
                  </>
                )}
              </div>
            ))}
          </div>
        ) : (
          <Empty
            description="No remote data references configured"
            image={Empty.PRESENTED_IMAGE_SIMPLE}
          >
            {accessibleWorkspaces.length > 0 && !showAddForm ? (
              <Button type="primary" onClick={() => setShowAddForm(true)}>
                Add Your First Reference
              </Button>
            ) : accessibleWorkspaces.length === 0 ? (
              <Text type="secondary">
                No workspaces are sharing their outputs with you.
              </Text>
            ) : null}
          </Empty>
        )}
      </Card>

      {/* Delete Confirmation Dialog */}
      <ConfirmDialog
        isOpen={deleteConfirm.show}
        title="Delete Remote Data Reference"
        message={`Are you sure you want to delete the remote data reference "${deleteConfirm.name}"? This action cannot be undone.`}
        confirmText="Delete"
        cancelText="Cancel"
        type="danger"
        loading={deleting}
        onConfirm={confirmDelete}
        onCancel={() => setDeleteConfirm({ show: false, id: '', name: '' })}
      />
    </div>
  );
};

export default WorkspaceRemoteDataConfig;
