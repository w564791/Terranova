import React, { useState, useEffect } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { workspaceService } from '../services/workspaces';
import styles from './EditWorkspace.module.css';

const EditWorkspace: React.FC = () => {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const [loading, setLoading] = useState(false);
  const [initialLoading, setInitialLoading] = useState(true);
  const [formData, setFormData] = useState({
    description: '',
    terraform_version: '1.5.0',
    execution_mode: 'server' as 'server' | 'agent' | 'k8s'
  });

  const loadWorkspace = async () => {
    if (!id) return;
    
    try {
      const response = await workspaceService.getWorkspace(parseInt(id));
      const workspace = response.data;
      setFormData({
        description: workspace.description || '',
        terraform_version: workspace.terraform_version || '1.5.0',
        execution_mode: workspace.execution_mode || 'server'
      });
    } catch (error) {
      console.error('加载工作空间失败:', error);
    } finally {
      setInitialLoading(false);
    }
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!id) return;
    
    setLoading(true);
    try {
      await workspaceService.updateWorkspace(parseInt(id), formData);
      navigate(`/workspaces/${id}`);
    } catch (error) {
      console.error('更新工作空间失败:', error);
    } finally {
      setLoading(false);
    }
  };

  const handleChange = (e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement>) => {
    setFormData({
      ...formData,
      [e.target.name]: e.target.value
    });
  };

  useEffect(() => {
    loadWorkspace();
  }, [id]);

  if (initialLoading) {
    return <div className={styles.loading}>加载中...</div>;
  }

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <button onClick={() => navigate(`/workspaces/${id}`)} className={styles.backButton}>
          ← 返回
        </button>
        <h1 className={styles.title}>编辑工作空间</h1>
      </div>

      <div className={styles.formContainer}>
        <form onSubmit={handleSubmit} className={styles.form}>
          <div className={styles.formGroup}>
            <label className={styles.label}>执行模式</label>
            <select
              name="execution_mode"
              value={formData.execution_mode}
              onChange={handleChange}
              className={styles.select}
            >
              <option value="server">Server - 平台直接执行</option>
              <option value="agent">Agent - 独立Agent执行</option>
              <option value="k8s">K8s - 动态Pod执行</option>
            </select>
          </div>

          <div className={styles.formGroup}>
            <label className={styles.label}>Terraform版本</label>
            <select
              name="terraform_version"
              value={formData.terraform_version}
              onChange={handleChange}
              className={styles.select}
            >
              <option value="1.5.0">1.5.0</option>
              <option value="1.4.6">1.4.6</option>
              <option value="1.3.9">1.3.9</option>
            </select>
          </div>

          <div className={styles.formGroup}>
            <label className={styles.label}>描述</label>
            <textarea
              name="description"
              value={formData.description}
              onChange={handleChange}
              className={styles.textarea}
              placeholder="输入工作空间描述"
              rows={4}
            />
          </div>

          <div className={styles.actions}>
            <button
              type="button"
              onClick={() => navigate(`/workspaces/${id}`)}
              className={styles.cancelButton}
            >
              取消
            </button>
            <button
              type="submit"
              disabled={loading}
              className={styles.submitButton}
            >
              {loading ? '保存中...' : '保存更改'}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
};

export default EditWorkspace;