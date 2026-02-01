import React, { useState, useEffect } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { agentPoolAPI, type AgentPool } from '../../services/agent';
import { useToast } from '../../contexts/ToastContext';
import styles from './AgentPoolForm.module.css';

const AgentPoolForm: React.FC = () => {
  const { poolId } = useParams<{ poolId: string }>();
  const [formData, setFormData] = useState({ 
    name: '', 
    description: '',
    pool_type: 'static' as 'static' | 'k8s'
  });
  const [loading, setLoading] = useState(false);
  const [initialLoading, setInitialLoading] = useState(!!poolId);
  const { showToast } = useToast();
  const navigate = useNavigate();
  const isEdit = !!poolId;

  useEffect(() => {
    if (poolId) {
      loadPool();
    }
  }, [poolId]);

  const loadPool = async () => {
    if (!poolId) return;
    
    try {
      setInitialLoading(true);
      const data = await agentPoolAPI.get(poolId);
      setFormData({
        name: data.pool.name,
        description: data.pool.description || '',
        pool_type: data.pool.pool_type || 'static',
      });
    } catch (error: any) {
      showToast(error.response?.data?.error || 'Failed to load pool', 'error');
      navigate('/global/settings/agent-pools');
    } finally {
      setInitialLoading(false);
    }
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();

    if (!formData.name.trim()) {
      showToast('Pool name is required', 'error');
      return;
    }

    try {
      setLoading(true);
      if (isEdit && poolId) {
        await agentPoolAPI.update(poolId, {
          name: formData.name,
          description: formData.description || undefined,
        });
        showToast('Agent pool updated successfully', 'success');
        navigate(`/global/settings/agent-pools/${poolId}`);
      } else {
        const newPool = await agentPoolAPI.create({
          name: formData.name,
          description: formData.description || undefined,
          pool_type: formData.pool_type,
        });
        showToast('Agent pool created successfully', 'success');
        navigate(`/global/settings/agent-pools/${newPool.pool_id}`);
      }
    } catch (error: any) {
      showToast(error.response?.data?.error || `Failed to ${isEdit ? 'update' : 'create'} pool`, 'error');
    } finally {
      setLoading(false);
    }
  };

  if (initialLoading) {
    return <div className={styles.loading}>Loading...</div>;
  }

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <button className={styles.backButton} onClick={() => navigate(-1)}>
          ← Back
        </button>
        <h1>{isEdit ? 'Edit Agent Pool' : 'Create Agent Pool'}</h1>
      </div>

      <form onSubmit={handleSubmit} className={styles.form}>
        <div className={styles.formGroup}>
          <label htmlFor="name">Pool Name *</label>
          <input
            id="name"
            type="text"
            value={formData.name}
            onChange={(e) => setFormData({ ...formData, name: e.target.value })}
            placeholder="Enter pool name"
            required
          />
        </div>

        <div className={styles.formGroup}>
          <label htmlFor="pool_type">Pool Type *</label>
          <select
            id="pool_type"
            value={formData.pool_type}
            onChange={(e) => setFormData({ ...formData, pool_type: e.target.value as 'static' | 'k8s' })}
            disabled={isEdit}
            required
          >
            <option value="static">Static Pool</option>
            <option value="k8s">Kubernetes Pool</option>
          </select>
          <small className={styles.helpText}>
            {isEdit 
              ? 'Pool type cannot be changed after creation'
              : 'Static pools use pre-registered agents, K8s pools create temporary agents on-demand'
            }
          </small>
        </div>

        <div className={styles.formGroup}>
          <label htmlFor="description">Description</label>
          <textarea
            id="description"
            value={formData.description}
            onChange={(e) => setFormData({ ...formData, description: e.target.value })}
            placeholder="Enter description (optional)"
            rows={4}
          />
        </div>

        <div className={styles.formActions}>
          <button 
            type="button" 
            className={styles.cancelButton}
            onClick={() => navigate(-1)}
            disabled={loading}
          >
            Cancel
          </button>
          <button 
            type="submit" 
            className={styles.submitButton}
            disabled={loading}
          >
            {loading ? 'Saving...' : (isEdit ? 'Update Pool' : 'Create Pool')}
          </button>
        </div>
      </form>
    </div>
  );
};

export default AgentPoolForm;
