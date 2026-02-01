import React, { useState, useEffect } from 'react';
import { useNavigate, Link } from 'react-router-dom';
import { agentPoolAPI, type AgentPoolWithCount } from '../../services/agent';
import { useToast } from '../../contexts/ToastContext';
import styles from './AgentPoolManagement.module.css';

const AgentPoolManagement: React.FC = () => {
  const [pools, setPools] = useState<AgentPoolWithCount[]>([]);
  const [loading, setLoading] = useState(true);
  const { showToast } = useToast();
  const navigate = useNavigate();

  useEffect(() => {
    loadPools();
  }, []);

  const loadPools = async () => {
    try {
      setLoading(true);
      const data = await agentPoolAPI.list();
      console.log('Agent pools data:', data);
      if (data && data.pools) {
        const poolsArray = Array.isArray(data.pools) ? data.pools : [];
        setPools(poolsArray);
      } else {
        setPools([]);
      }
    } catch (error: any) {
      console.error('Failed to load agent pools:', error);
      showToast(error.response?.data?.error || 'Failed to load agent pools', 'error');
      setPools([]);
    } finally {
      setLoading(false);
    }
  };

  const getStatusBadge = (status: string) => {
    const statusMap: Record<string, { label: string; className: string }> = {
      active: { label: 'Active', className: styles.statusActive },
      inactive: { label: 'Inactive', className: styles.statusInactive },
      maintenance: { label: 'Maintenance', className: styles.statusMaintenance },
    };
    const config = statusMap[status] || { label: status || 'N/A', className: '' };
    return <span className={`${styles.statusBadge} ${config.className}`}>{config.label}</span>;
  };

  if (loading) {
    return <div className={styles.loading}>Loading agent pools...</div>;
  }

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <h1>Agent Pool Management</h1>
        <button 
          className={styles.createButton} 
          onClick={() => navigate('/global/settings/agent-pools/create')}
        >
          + Create Agent Pool
        </button>
      </div>

      <div className={styles.poolList}>
        {pools.length === 0 ? (
          <div className={styles.emptyState}>
            <p>No agent pools found</p>
            <button onClick={() => navigate('/global/settings/agent-pools/create')}>
              Create your first agent pool
            </button>
          </div>
        ) : (
          <table className={styles.table}>
            <thead>
              <tr>
                <th>Pool Name</th>
                <th>Description</th>
                <th>Type</th>
                <th>Status</th>
                <th>Agents</th>
                <th>Max Agents</th>
                <th>Created</th>
              </tr>
            </thead>
            <tbody>
              {pools.map((pool) => (
                <tr 
                  key={pool.pool_id}
                  className={styles.clickableRow}
                  onClick={() => navigate(`/global/settings/agent-pools/${pool.pool_id}`)}
                >
                  <td className={styles.poolName}>
                    <Link 
                      to={`/global/settings/agent-pools/${pool.pool_id}`}
                      onClick={(e) => e.stopPropagation()}
                      className={styles.poolNameLink}
                    >
                      {pool.name}
                    </Link>
                  </td>
                  <td className={styles.description}>{pool.description || '-'}</td>
                  <td>{pool.pool_type}</td>
                  <td>{getStatusBadge(pool.status)}</td>
                  <td className={styles.agentCount}>{pool.agent_count}</td>
                  <td>{pool.max_agents}</td>
                  <td>{new Date(pool.created_at).toLocaleDateString()}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  );
};

export default AgentPoolManagement;
