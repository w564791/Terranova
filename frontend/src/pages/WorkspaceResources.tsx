import React, { useState, useEffect } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import styles from './WorkspaceResources.module.css';
import { useToast } from '../contexts/ToastContext';
import api from '../services/api';

interface Resource {
  id: number;
  resource_id: string;
  resource_type: string;
  resource_name: string;
  description: string;
  is_active: boolean;
  current_version?: {
    version: number;
    change_summary: string;
    created_at: string;
  };
  created_at: string;
}

const WorkspaceResources: React.FC = () => {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const { showToast } = useToast();
  
  const [resources, setResources] = useState<Resource[]>([]);
  const [loading, setLoading] = useState(true);
  const [selectedResources, setSelectedResources] = useState<number[]>([]);
  const [showAddDialog, setShowAddDialog] = useState(false);

  useEffect(() => {
    loadResources();
  }, [id]);

  const loadResources = async () => {
    try {
      setLoading(true);
      const response = await api.get(`/workspaces/${id}/resources`);
      setResources(response.data.resources || []);
    } catch (error: any) {
      showToast(error.response?.data?.error || '加载资源失败', 'error');
    } finally {
      setLoading(false);
    }
  };

  const handleSelectResource = (resourceId: number) => {
    setSelectedResources(prev => {
      if (prev.includes(resourceId)) {
        return prev.filter(id => id !== resourceId);
      } else {
        return [...prev, resourceId];
      }
    });
  };

  const handleSelectAll = () => {
    if (selectedResources.length === resources.length) {
      setSelectedResources([]);
    } else {
      setSelectedResources(resources.map(r => r.id));
    }
  };

  const handleDeploySelected = async () => {
    if (selectedResources.length === 0) {
      showToast('请选择要部署的资源', 'warning');
      return;
    }

    try {
      const response = await api.post(`/workspaces/${id}/resources/deploy`, {
        resource_ids: selectedResources
      });
      showToast(response.data.message, 'success');
      setSelectedResources([]);
    } catch (error: any) {
      showToast(error.response?.data?.error || '部署失败', 'error');
    }
  };

  const handleCreateSnapshot = async () => {
    const snapshotName = prompt('请输入快照名称:');
    if (!snapshotName) return;

    const description = prompt('请输入快照描述（可选）:');

    try {
      const response = await api.post(`/workspaces/${id}/snapshots`, {
        snapshot_name: snapshotName,
        description: description || ''
      });
      showToast(response.data.message, 'success');
    } catch (error: any) {
      showToast(error.response?.data?.error || '创建快照失败', 'error');
    }
  };

  const handleViewVersions = (resourceId: number) => {
    navigate(`/workspaces/${id}/resources/${resourceId}/versions`);
  };

  const handleDeleteResource = async (resourceId: number) => {
    if (!confirm('确定要删除这个资源吗？')) return;

    try {
      await api.delete(`/workspaces/${id}/resources/${resourceId}`);
      showToast('资源已删除', 'success');
      loadResources();
    } catch (error: any) {
      showToast(error.response?.data?.error || '删除失败', 'error');
    }
  };

  if (loading) {
    return <div className={styles.loading}>加载中...</div>;
  }

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <div className={styles.headerLeft}>
          <h1>资源管理</h1>
          <span className={styles.count}>共 {resources.length} 个资源</span>
        </div>
        <div className={styles.headerRight}>
          <button 
            className={styles.btnSecondary}
            onClick={handleCreateSnapshot}
          >
            创建快照
          </button>
          <button 
            className={styles.btnPrimary}
            onClick={() => setShowAddDialog(true)}
          >
            + 添加资源
          </button>
        </div>
      </div>

      {selectedResources.length > 0 && (
        <div className={styles.actionBar}>
          <span>已选择 {selectedResources.length} 个资源</span>
          <div className={styles.actionButtons}>
            <button 
              className={styles.btnDeploy}
              onClick={handleDeploySelected}
            >
              部署选定资源
            </button>
            <button 
              className={styles.btnCancel}
              onClick={() => setSelectedResources([])}
            >
              取消选择
            </button>
          </div>
        </div>
      )}

      <div className={styles.resourceList}>
        <div className={styles.listHeader}>
          <input
            type="checkbox"
            checked={selectedResources.length === resources.length && resources.length > 0}
            onChange={handleSelectAll}
          />
          <span>资源标识</span>
          <span>类型</span>
          <span>版本</span>
          <span>最后变更</span>
          <span>操作</span>
        </div>

        {resources.length === 0 ? (
          <div className={styles.empty}>
            <p>暂无资源</p>
            <button 
              className={styles.btnPrimary}
              onClick={() => setShowAddDialog(true)}
            >
              添加第一个资源
            </button>
          </div>
        ) : (
          resources.map(resource => (
            <div key={resource.id} className={styles.resourceItem}>
              <input
                type="checkbox"
                checked={selectedResources.includes(resource.id)}
                onChange={() => handleSelectResource(resource.id)}
              />
              <div className={styles.resourceInfo}>
                <div className={styles.resourceId}>{resource.resource_id}</div>
                <div className={styles.description}>{resource.description}</div>
              </div>
              <div className={styles.resourceType}>{resource.resource_type}</div>
              <div className={styles.version}>
                {resource.current_version ? (
                  <>
                    <span className={styles.versionNumber}>v{resource.current_version.version}</span>
                    <span className={styles.versionSummary}>{resource.current_version.change_summary}</span>
                  </>
                ) : (
                  <span className={styles.noVersion}>-</span>
                )}
              </div>
              <div className={styles.timestamp}>
                {new Date(resource.created_at).toLocaleString('zh-CN')}
              </div>
              <div className={styles.actions}>
                <button 
                  className={styles.btnAction}
                  onClick={() => handleViewVersions(resource.id)}
                >
                  版本历史
                </button>
                <button 
                  className={styles.btnAction}
                  onClick={() => navigate(`/workspaces/${id}/resources/${resource.id}/edit`)}
                >
                  编辑
                </button>
                <button 
                  className={styles.btnDanger}
                  onClick={() => handleDeleteResource(resource.id)}
                >
                  删除
                </button>
              </div>
            </div>
          ))
        )}
      </div>

      {showAddDialog && (
        <AddResourceDialog
          workspaceId={id!}
          onClose={() => setShowAddDialog(false)}
          onSuccess={() => {
            setShowAddDialog(false);
            loadResources();
          }}
        />
      )}
    </div>
  );
};

// 添加资源对话框组件
const AddResourceDialog: React.FC<{
  workspaceId: string;
  onClose: () => void;
  onSuccess: () => void;
}> = ({ workspaceId, onClose, onSuccess }) => {
  const { showToast } = useToast();
  const [formData, setFormData] = useState({
    resource_type: '',
    resource_name: '',
    description: '',
    tf_code: '{\n  "resource": {\n    "": {\n      "": {\n        \n      }\n    }\n  }\n}'
  });

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();

    try {
      const tfCode = JSON.parse(formData.tf_code);
      
      await api.post(`/workspaces/${workspaceId}/resources`, {
        resource_type: formData.resource_type,
        resource_name: formData.resource_name,
        description: formData.description,
        tf_code: tfCode
      });

      showToast('资源添加成功', 'success');
      onSuccess();
    } catch (error: any) {
      showToast(error.response?.data?.error || '添加资源失败', 'error');
    }
  };

  return (
    <div className={styles.dialogOverlay} onClick={onClose}>
      <div className={styles.dialog} onClick={e => e.stopPropagation()}>
        <div className={styles.dialogHeader}>
          <h2>添加资源</h2>
          <button className={styles.closeBtn} onClick={onClose}>×</button>
        </div>
        
        <form onSubmit={handleSubmit} className={styles.dialogContent}>
          <div className={styles.formGroup}>
            <label>资源类型 *</label>
            <input
              type="text"
              value={formData.resource_type}
              onChange={e => setFormData({...formData, resource_type: e.target.value})}
              placeholder="例如: aws_s3_bucket"
              required
            />
          </div>

          <div className={styles.formGroup}>
            <label>资源名称 *</label>
            <input
              type="text"
              value={formData.resource_name}
              onChange={e => setFormData({...formData, resource_name: e.target.value})}
              placeholder="例如: my_bucket"
              required
            />
          </div>

          <div className={styles.formGroup}>
            <label>描述</label>
            <input
              type="text"
              value={formData.description}
              onChange={e => setFormData({...formData, description: e.target.value})}
              placeholder="资源描述"
            />
          </div>

          <div className={styles.formGroup}>
            <label>Terraform配置 (JSON) *</label>
            <textarea
              value={formData.tf_code}
              onChange={e => setFormData({...formData, tf_code: e.target.value})}
              rows={10}
              className={styles.codeEditor}
              required
            />
          </div>

          <div className={styles.dialogFooter}>
            <button type="button" className={styles.btnCancel} onClick={onClose}>
              取消
            </button>
            <button type="submit" className={styles.btnSubmit}>
              添加资源
            </button>
          </div>
        </form>
      </div>
    </div>
  );
};

export default WorkspaceResources;
