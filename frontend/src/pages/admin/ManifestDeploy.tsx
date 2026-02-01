import React, { useState, useEffect } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import {
  Card,
  Button,
  Space,
  Typography,
  Spin,
  Form,
  Select,
  Tag,
  Divider,
  Alert,
} from 'antd';
import {
  ArrowLeftOutlined,
  RocketOutlined,
  WarningOutlined,
} from '@ant-design/icons';
import { useToast } from '../../contexts/ToastContext';
import ConfirmDialog from '../../components/ConfirmDialog';
import {
  getManifest,
  listManifestVersions,
  listManifestDeployments,
  createManifestDeployment,
  deleteManifestDeployment,
  getManifestDeploymentResources,
} from '../../services/manifestApi';
import type { Manifest, ManifestVersion, ManifestDeployment, DeploymentResource } from '../../services/manifestApi';
import styles from './ManifestDeploy.module.css';

const { Title, Text } = Typography;

const ManifestDeploy: React.FC = () => {
  const { id: manifestId } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const toast = useToast();
  const [form] = Form.useForm();

  const [loading, setLoading] = useState(true);
  const [manifest, setManifest] = useState<Manifest | null>(null);
  const [versions, setVersions] = useState<ManifestVersion[]>([]);
  const [deployments, setDeployments] = useState<ManifestDeployment[]>([]);
  const [workspaces, setWorkspaces] = useState<any[]>([]);
  const [deploying, setDeploying] = useState(false);
  const [deploymentResources, setDeploymentResources] = useState<Record<string, DeploymentResource[]>>({});
  const [loadingResources, setLoadingResources] = useState<Record<string, boolean>>({});
  const [expandedDeployments, setExpandedDeployments] = useState<Set<string>>(new Set());
  
  // 卸载确认对话框状态
  const [uninstallDialog, setUninstallDialog] = useState<{
    isOpen: boolean;
    deployment: ManifestDeployment | null;
    resources: DeploymentResource[];
    driftedResources: DeploymentResource[];
    confirmName: string;
    loading: boolean;
  }>({
    isOpen: false,
    deployment: null,
    resources: [],
    driftedResources: [],
    confirmName: '',
    loading: false,
  });

  const orgId = manifest?.organization_id?.toString() || '1';

  // 加载数据
  useEffect(() => {
    const loadData = async () => {
      if (!manifestId) return;
      
      setLoading(true);
      try {
        const manifestData = await getManifest('1', manifestId);
        setManifest(manifestData);

        const versionsResponse = await listManifestVersions(manifestData.organization_id?.toString() || '1', manifestId);
        const publishedVersions = (versionsResponse.items || []).filter(v => v.is_draft === false);
        setVersions(publishedVersions);

        const deploymentsResponse = await listManifestDeployments(manifestData.organization_id?.toString() || '1', manifestId);
        setDeployments(deploymentsResponse.items || []);

        const wsResponse = await fetch('/api/v1/workspaces', {
          headers: { 'Authorization': `Bearer ${localStorage.getItem('token')}` },
        });
        if (wsResponse.ok) {
          const wsData = await wsResponse.json();
          let wsList: any[] = [];
          if (Array.isArray(wsData)) wsList = wsData;
          else if (wsData.data?.items) wsList = wsData.data.items;
          else if (wsData.data && Array.isArray(wsData.data)) wsList = wsData.data;
          else if (wsData.items) wsList = wsData.items;
          setWorkspaces(wsList);
        }
      } catch (error: any) {
        toast.error('加载数据失败: ' + (error.message || '未知错误'));
      } finally {
        setLoading(false);
      }
    };
    loadData();
  }, [manifestId]);

  // 加载部署资源
  const loadDeploymentResources = async (deploymentId: string) => {
    if (deploymentResources[deploymentId] || loadingResources[deploymentId]) return;

    const currentOrgId = manifest?.organization_id?.toString() || '1';
    console.log('[ManifestDeploy] Loading resources for deployment:', deploymentId, 'orgId:', currentOrgId, 'manifestId:', manifestId);
    
    setLoadingResources(prev => ({ ...prev, [deploymentId]: true }));
    try {
      const resources = await getManifestDeploymentResources(currentOrgId, manifestId!, deploymentId);
      console.log('[ManifestDeploy] Loaded resources:', resources);
      setDeploymentResources(prev => ({ ...prev, [deploymentId]: resources || [] }));
    } catch (error: any) {
      console.error('加载部署资源失败:', error);
      setDeploymentResources(prev => ({ ...prev, [deploymentId]: [] }));
    } finally {
      setLoadingResources(prev => ({ ...prev, [deploymentId]: false }));
    }
  };

  // 切换展开状态
  const toggleDeployment = (deploymentId: string) => {
    setExpandedDeployments(prev => {
      const next = new Set(prev);
      if (next.has(deploymentId)) {
        next.delete(deploymentId);
      } else {
        next.add(deploymentId);
        loadDeploymentResources(deploymentId);
      }
      return next;
    });
  };

  // 处理部署
  const handleDeploy = async (values: any) => {
    if (!manifestId) return;
    setDeploying(true);
    try {
      await createManifestDeployment(orgId, manifestId, {
        version_id: values.version_id,
        workspace_id: values.workspace_id,
        auto_apply: values.auto_apply || false,
      });
      toast.success('部署任务已创建');
      const deploymentsResponse = await listManifestDeployments(orgId, manifestId);
      setDeployments(deploymentsResponse.items || []);
      form.resetFields();
    } catch (error: any) {
      toast.error('部署失败: ' + (error.message || '未知错误'));
    } finally {
      setDeploying(false);
    }
  };

  // 处理废弃
  const handleArchive = async (deployment: ManifestDeployment) => {
    try {
      await deleteManifestDeployment(orgId, manifestId!, deployment.id);
      toast.success('部署已废弃');
      const deploymentsResponse = await listManifestDeployments(orgId, manifestId!);
      setDeployments(deploymentsResponse.items || []);
    } catch (error: any) {
      toast.error('废弃失败: ' + (error.message || '未知错误'));
    }
  };

  // 处理卸载 - 打开确认对话框
  const handleUninstall = async (deployment: ManifestDeployment) => {
    console.log('[ManifestDeploy] handleUninstall called for deployment:', deployment.id);
    
    // 先加载资源（如果还没加载）
    let resources = deploymentResources[deployment.id];
    if (!resources) {
      console.log('[ManifestDeploy] Loading resources first...');
      try {
        const currentOrgId = manifest?.organization_id?.toString() || '1';
        resources = await getManifestDeploymentResources(currentOrgId, manifestId!, deployment.id);
        setDeploymentResources(prev => ({ ...prev, [deployment.id]: resources || [] }));
      } catch (error) {
        console.error('[ManifestDeploy] Failed to load resources:', error);
        resources = [];
        setDeploymentResources(prev => ({ ...prev, [deployment.id]: [] }));
      }
    }
    
    const driftedResources = resources.filter(r => r.is_drifted);
    
    // 打开确认对话框
    setUninstallDialog({
      isOpen: true,
      deployment,
      resources,
      driftedResources,
      confirmName: '',
      loading: false,
    });
  };

  // 执行卸载
  const executeUninstall = async () => {
    const { deployment, driftedResources } = uninstallDialog;
    if (!deployment) return;
    
    setUninstallDialog(prev => ({ ...prev, loading: true }));
    
    try {
      const force = driftedResources.length > 0;
      const result = await deleteManifestDeployment(orgId, manifestId!, deployment.id, { uninstall: true, force });
      toast.success(`卸载任务已创建，任务 ID: ${result.task_id}`);
      const deploymentsResponse = await listManifestDeployments(orgId, manifestId!);
      setDeployments(deploymentsResponse.items || []);
      setUninstallDialog(prev => ({ ...prev, isOpen: false }));
    } catch (error: any) {
      toast.error('卸载失败: ' + (error.message || '未知错误'));
      setUninstallDialog(prev => ({ ...prev, loading: false }));
    }
  };

  // 获取状态配置
  const getStatusConfig = (status: string) => {
    const configs: Record<string, { icon: string; color: string; bgColor: string; borderColor: string; label: string }> = {
      pending: { icon: '○', color: '#6b7280', bgColor: '#f3f4f6', borderColor: '#6b7280', label: '等待中' },
      deploying: { icon: '◐', color: '#3b82f6', bgColor: '#dbeafe', borderColor: '#3b82f6', label: '部署中' },
      deployed: { icon: '✓', color: '#10b981', bgColor: '#d1fae5', borderColor: '#10b981', label: '已部署' },
      failed: { icon: '✗', color: '#ef4444', bgColor: '#fee2e2', borderColor: '#ef4444', label: '失败' },
      archived: { icon: '⊘', color: '#6b7280', bgColor: '#f3f4f6', borderColor: '#6b7280', label: '已废弃' },
    };
    return configs[status] || configs.pending;
  };

  // 格式化时间
  const formatRelativeTime = (dateString?: string) => {
    if (!dateString) return '';
    const date = new Date(dateString);
    const now = new Date();
    const diffMs = now.getTime() - date.getTime();
    const diffMins = Math.floor(diffMs / 60000);
    const diffHours = Math.floor(diffMs / 3600000);
    if (diffMins < 1) return '刚刚';
    if (diffMins < 60) return `${diffMins} 分钟前`;
    if (diffHours < 24) return `${diffHours} 小时前`;
    return date.toLocaleString();
  };

  // 渲染部署卡片
  const renderDeploymentCard = (deployment: ManifestDeployment) => {
    const statusConfig = getStatusConfig(deployment.status);
    const isExpanded = expandedDeployments.has(deployment.id);
    const resources = deploymentResources[deployment.id] || [];
    const isLoadingRes = loadingResources[deployment.id];
    const driftedCount = resources.filter(r => r.is_drifted).length;

    return (
      <div 
        key={deployment.id} 
        className={styles.deploymentCard}
        style={{ borderLeftColor: statusConfig.borderColor }}
      >
        <div 
          className={`${styles.deploymentHeader} ${isExpanded ? styles.deploymentHeaderExpanded : ''}`}
          onClick={() => toggleDeployment(deployment.id)}
        >
          <div className={styles.deploymentTitle}>
            <span 
              className={styles.statusIcon}
              style={{ color: statusConfig.color, backgroundColor: statusConfig.bgColor }}
            >
              {statusConfig.icon}
            </span>
            <span className={styles.titleText}>
              <strong>{deployment.workspace_name}</strong>
              <span className={styles.workspaceId}>({deployment.workspace_semantic_id})</span>
            </span>
            <span className={styles.deploymentTime}>
              {formatRelativeTime(deployment.deployed_at)}
            </span>
          </div>
          <div className={styles.deploymentSummary}>
            <Tag color="blue">{deployment.version_name}</Tag>
            {driftedCount > 0 && (
              <Tag color="warning" icon={<WarningOutlined />}>
                {driftedCount} 漂移
              </Tag>
            )}
            <span className={styles.expandButton}>
              {isExpanded ? '∧' : '∨'}
            </span>
          </div>
        </div>
        
        {isExpanded && (
          <div className={styles.deploymentContent}>
            {/* 详情网格 */}
            <div className={styles.detailsGrid}>
              <div className={styles.detailItem}>
                <span className={styles.detailLabel}>状态</span>
                <span className={styles.detailValue}>{statusConfig.label}</span>
              </div>
              <div className={styles.detailItem}>
                <span className={styles.detailLabel}>版本</span>
                <span className={styles.detailValue}>{deployment.version_name}</span>
              </div>
              <div className={styles.detailItem}>
                <span className={styles.detailLabel}>资源数</span>
                <span className={styles.detailValue}>{resources.length}</span>
              </div>
              <div className={styles.detailItem}>
                <span className={styles.detailLabel}>部署时间</span>
                <span className={styles.detailValue}>
                  {deployment.deployed_at ? new Date(deployment.deployed_at).toLocaleString() : '-'}
                </span>
              </div>
            </div>

            {/* 资源列表 */}
            <div className={styles.resourcesSection}>
              <div className={styles.resourcesTitle}>关联资源</div>
              {isLoadingRes ? (
                <div className={styles.resourcesLoading}>
                  <Spin size="small" /> 加载中...
                </div>
              ) : resources.length === 0 ? (
                <div className={styles.resourcesEmpty}>暂无关联资源</div>
              ) : (
                <div className={styles.resourcesList}>
                  {resources.map(resource => (
                    <div key={resource.resource_id} className={styles.resourceItem}>
                      <div className={styles.resourceMain}>
                        {resource.is_drifted && (
                          <span className={styles.driftBadge} title="资源已漂移">⚠</span>
                        )}
                        {resource.resource_db_id > 0 ? (
                          <a 
                            className={styles.resourceLink}
                            onClick={(e) => {
                              e.stopPropagation();
                              navigate(`/workspaces/${deployment.workspace_semantic_id}/resources/${resource.resource_db_id}`);
                            }}
                          >
                            {resource.resource_name}
                          </a>
                        ) : (
                          <span className={styles.resourceName}>{resource.resource_name}</span>
                        )}
                        <span className={styles.resourceId}>{resource.resource_id}</span>
                      </div>
                      <div className={styles.resourceMeta}>
                        <span className={styles.resourceType}>{resource.resource_type}</span>
                        <span className={`${styles.resourceStatus} ${resource.is_active ? styles.active : styles.inactive}`}>
                          {resource.is_active ? '活跃' : '已删除'}
                        </span>
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </div>

            {/* 操作按钮 */}
            {deployment.status !== 'archived' && (
              <div className={styles.deploymentActions}>
                <Button 
                  size="small"
                  onClick={(e) => {
                    e.stopPropagation();
                    navigate(`/workspaces/${deployment.workspace_semantic_id}`);
                  }}
                >
                  查看 Workspace
                </Button>
                <Button 
                  size="small"
                  onClick={(e) => {
                    e.stopPropagation();
                    handleArchive(deployment);
                  }}
                >
                  废弃
                </Button>
                <Button 
                  size="small"
                  danger
                  onClick={(e) => {
                    e.stopPropagation();
                    handleUninstall(deployment);
                  }}
                >
                  卸载
                </Button>
              </div>
            )}
          </div>
        )}
      </div>
    );
  };

  if (loading) {
    return (
      <div className={styles.loadingContainer}>
        <Spin size="large" />
        <div style={{ marginTop: 16 }}>加载中...</div>
      </div>
    );
  }

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <Space>
          <Button icon={<ArrowLeftOutlined />} onClick={() => navigate('/admin/manifests')}>
            返回
          </Button>
          <Divider type="vertical" />
          <Title level={4} style={{ margin: 0 }}>
            部署 Manifest: {manifest?.name}
          </Title>
        </Space>
        <Space>
          <Button onClick={() => navigate(`/admin/manifests/${manifestId}/edit`)}>
            编辑 Manifest
          </Button>
        </Space>
      </div>

      <div className={styles.content}>
        {/* 部署表单 */}
        <Card title="创建新部署" className={styles.deployCard}>
          {versions.length === 0 ? (
            <Alert
              type="warning"
              message="暂无可部署的版本"
              description="请先在编辑器中发布一个版本，然后才能进行部署。"
              action={
                <Button type="primary" onClick={() => navigate(`/admin/manifests/${manifestId}/edit`)}>
                  去编辑
                </Button>
              }
            />
          ) : (
            <Form form={form} layout="vertical" onFinish={handleDeploy}>
              <Form.Item
                name="version_id"
                label="选择版本"
                rules={[{ required: true, message: '请选择要部署的版本' }]}
              >
                <Select
                  placeholder="选择要部署的版本"
                  options={versions.map(v => ({
                    label: `${v.version} (${new Date(v.created_at).toLocaleDateString()})`,
                    value: v.id,
                  }))}
                />
              </Form.Item>

              <Form.Item
                name="workspace_id"
                label="目标 Workspace"
                rules={[{ required: true, message: '请选择目标 Workspace' }]}
              >
                <Select
                  placeholder="选择要部署到的 Workspace"
                  showSearch
                  optionFilterProp="label"
                  options={(Array.isArray(workspaces) ? workspaces : []).map(ws => ({
                    label: ws.name,
                    value: ws.id,
                  }))}
                />
              </Form.Item>

              <Form.Item>
                <Button
                  type="primary"
                  htmlType="submit"
                  icon={<RocketOutlined />}
                  loading={deploying}
                >
                  开始部署
                </Button>
              </Form.Item>
            </Form>
          )}
        </Card>

        {/* 部署历史 */}
        <div className={styles.historySection}>
          <div className={styles.historyTitle}>部署历史</div>
          {deployments.length === 0 ? (
            <div className={styles.emptyDeployments}>暂无部署记录</div>
          ) : (
            <div className={styles.deploymentsList}>
              {deployments.map(renderDeploymentCard)}
            </div>
          )}
        </div>
      </div>

      {/* 卸载确认对话框 */}
      <ConfirmDialog
        isOpen={uninstallDialog.isOpen}
        title={uninstallDialog.driftedResources.length > 0 ? '⚠️ 检测到漂移资源' : '确认卸载部署'}
        type="danger"
        confirmText={uninstallDialog.loading ? '卸载中...' : '确认卸载'}
        cancelText="取消"
        loading={uninstallDialog.loading}
        confirmDisabled={uninstallDialog.confirmName !== uninstallDialog.deployment?.workspace_name}
        onConfirm={executeUninstall}
        onCancel={() => setUninstallDialog(prev => ({ ...prev, isOpen: false, confirmName: '' }))}
      >
        <div className={styles.uninstallDialogContent}>
          {uninstallDialog.driftedResources.length > 0 && (
            <div className={styles.driftWarning}>
              <p>以下资源已被手动修改：</p>
              <ul className={styles.driftedList}>
                {uninstallDialog.driftedResources.map(r => (
                  <li key={r.resource_id}>{r.resource_name}</li>
                ))}
              </ul>
              <p className={styles.warningText}>
                强制卸载将删除这些资源，包括手动修改的部分。
              </p>
            </div>
          )}
          
          <div className={styles.resourceSummary}>
            <p>卸载将永久删除 <strong>{uninstallDialog.resources.length}</strong> 个资源：</p>
            <ul className={styles.resourceList}>
              {uninstallDialog.resources.slice(0, 5).map(r => (
                <li key={r.resource_id}>{r.resource_name}</li>
              ))}
              {uninstallDialog.resources.length > 5 && (
                <li>...等 {uninstallDialog.resources.length - 5} 个资源</li>
              )}
            </ul>
          </div>

          <div className={styles.confirmSection}>
            <p className={styles.confirmLabel}>
              请输入 Workspace 名称 <strong>{uninstallDialog.deployment?.workspace_name}</strong> 以确认卸载：
            </p>
            <input
              type="text"
              className={styles.confirmInput}
              placeholder={uninstallDialog.deployment?.workspace_name || ''}
              value={uninstallDialog.confirmName}
              onChange={(e) => setUninstallDialog(prev => ({ ...prev, confirmName: e.target.value }))}
            />
          </div>

          <p className={styles.dangerText}>此操作不可撤销！</p>
        </div>
      </ConfirmDialog>
    </div>
  );
};

export default ManifestDeploy;
