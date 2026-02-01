import React, { useState, useEffect } from 'react';
import { workspacePoolAPI, agentPoolAPI, type PoolWithAgentCount, type CurrentPoolResponse, type Agent } from '../services/agent';
import { useToast } from '../contexts/ToastContext';
import styles from './WorkspaceAgentConfig.module.css';

interface WorkspaceAgentConfigProps {
  workspaceId: string;
}

const WorkspaceAgentConfig: React.FC<WorkspaceAgentConfigProps> = ({ workspaceId }) => {
  const [availablePools, setAvailablePools] = useState<PoolWithAgentCount[]>([]);
  const [currentPool, setCurrentPool] = useState<CurrentPoolResponse | null>(null);
  const [poolAgents, setPoolAgents] = useState<Agent[]>([]);
  const [loading, setLoading] = useState(true);
  const [showAgentsDialog, setShowAgentsDialog] = useState(false);
  const { showToast } = useToast();

  useEffect(() => {
    loadData();
  }, [workspaceId]);

  const loadData = async () => {
    try {
      setLoading(true);
      
      // Load available pools
      const poolsData = await workspacePoolAPI.getAvailablePools(workspaceId);
      setAvailablePools(poolsData.pools);

      // Load current pool
      try {
        const currentData = await workspacePoolAPI.getCurrentPool(workspaceId);
        setCurrentPool(currentData);
        
        // Load agents in current pool
        if (currentData.pool.pool_id) {
          loadPoolAgents(currentData.pool.pool_id);
        }
      } catch (error: any) {
        // No current pool set is okay
        if (error.response?.status !== 404) {
          console.error('Failed to load current pool:', error);
        }
      }
    } catch (error: any) {
      showToast(error.response?.data?.error || 'Failed to load pool configuration', 'error');
    } finally {
      setLoading(false);
    }
  };

  const loadPoolAgents = async (poolId: string) => {
    try {
      const data = await agentPoolAPI.get(poolId);
      setPoolAgents(data.agents || []);
    } catch (error: any) {
      console.error('Failed to load pool agents:', error);
    }
  };

  const handleSetCurrentPool = async (poolId: string) => {
    try {
      await workspacePoolAPI.setCurrentPool(workspaceId, poolId);
      showToast('Current pool set successfully', 'success');
      loadData();
    } catch (error: any) {
      showToast(error.response?.data?.error || 'Failed to set current pool', 'error');
    }
  };

  const handleViewAgents = async (poolId: string) => {
    try {
      const data = await agentPoolAPI.get(poolId);
      setPoolAgents(data.agents || []);
      setShowAgentsDialog(true);
    } catch (error: any) {
      showToast('Failed to load pool agents', 'error');
    }
  };

  const getStatusBadge = (status: string) => {
    const statusMap: Record<string, { label: string; className: string }> = {
      active: { label: 'Active', className: styles.statusActive },
      idle: { label: 'Idle', className: styles.statusIdle },
      busy: { label: 'Busy', className: styles.statusBusy },
      offline: { label: 'Offline', className: styles.statusOffline },
    };
    const config = statusMap[status] || { label: status, className: '' };
    return <span className={`${styles.statusBadge} ${config.className}`}>{config.label}</span>;
  };

  if (loading) {
    return <div className={styles.loading}>Loading pool configuration...</div>;
  }

  return (
    <div className={styles.container}>
      {/* Current Pool Section */}
      <div className={styles.section}>
        <h3>Current Agent Pool</h3>
        {currentPool ? (
          <div className={styles.currentPool}>
            <div className={styles.poolInfo}>
              <div className={styles.poolName}>
                {currentPool.pool.name}
                <span className={`${styles.poolTypeBadge} ${styles[`poolType${currentPool.pool.pool_type === 'k8s' ? 'K8s' : 'Static'}`]}`}>
                  {currentPool.pool.pool_type === 'k8s' ? 'K8s Pool' : 'Static Pool'}
                </span>
              </div>
              <div className={styles.poolId}>{currentPool.pool.pool_id}</div>
              {currentPool.pool.description && (
                <div className={styles.poolDesc}>{currentPool.pool.description}</div>
              )}
              <div className={styles.poolMeta}>
                <span className={styles.agentCount}>
                  {currentPool.pool.agent_count} agent{currentPool.pool.agent_count !== 1 ? 's' : ''}
                </span>
                <span className={styles.onlineCount}>
                  {currentPool.pool.online_count} online
                </span>
              </div>
            </div>
            <div className={styles.poolActions}>
              <button
                className={styles.viewAgentsButton}
                onClick={() => handleViewAgents(currentPool.pool.pool_id)}
              >
                View Agents
              </button>
            </div>
          </div>
        ) : (
          <div className={styles.noPool}>
            <p>No pool currently assigned to this workspace</p>
            <p className={styles.hint}>Select a pool from the available pools below</p>
          </div>
        )}
      </div>

      {/* Available Pools Section */}
      <div className={styles.section}>
        <h3>Available Pools ({availablePools.length})</h3>
        {availablePools.length === 0 ? (
          <div className={styles.emptyState}>
            <p>No pools have granted access to this workspace yet</p>
            <p className={styles.hint}>Pool administrators must first allow this workspace before it can be used</p>
          </div>
        ) : (
          <div className={styles.poolList}>
            {availablePools.map((pool) => {
              const isCurrent = currentPool?.pool.pool_id === pool.pool_id;
              return (
                <div key={pool.pool_id} className={`${styles.poolCard} ${isCurrent ? styles.currentCard : ''}`}>
                  <div className={styles.poolInfo}>
                    <div className={styles.poolName}>
                      {pool.name}
                      {isCurrent && <span className={styles.currentBadge}>Current</span>}
                      <span className={`${styles.poolTypeBadge} ${styles[`poolType${pool.pool_type === 'k8s' ? 'K8s' : 'Static'}`]}`}>
                        {pool.pool_type === 'k8s' ? 'K8s Pool' : 'Static Pool'}
                      </span>
                    </div>
                    <div className={styles.poolId}>{pool.pool_id}</div>
                    {pool.description && (
                      <div className={styles.poolDesc}>{pool.description}</div>
                    )}
                    <div className={styles.poolMeta}>
                      <span className={styles.agentCount}>
                        {pool.agent_count} agent{pool.agent_count !== 1 ? 's' : ''}
                      </span>
                      <span className={styles.onlineCount}>
                        {pool.online_count} online
                      </span>
                    </div>
                    <div className={styles.allowedAt}>
                      Allowed: {new Date(pool.allowed_at).toLocaleString()}
                    </div>
                  </div>
                  <div className={styles.poolActions}>
                    {!isCurrent && (
                      <button
                        className={styles.setCurrentButton}
                        onClick={() => handleSetCurrentPool(pool.pool_id)}
                        disabled={pool.online_count === 0}
                        title={pool.online_count === 0 ? 'No online agents in this pool' : 'Set as current pool'}
                      >
                        Set as Current
                      </button>
                    )}
                    <button
                      className={styles.viewAgentsButton}
                      onClick={() => handleViewAgents(pool.pool_id)}
                    >
                      View Agents ({pool.agent_count})
                    </button>
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </div>

      {/* Agents Dialog */}
      {showAgentsDialog && (
        <div className={styles.dialogOverlay} onClick={() => setShowAgentsDialog(false)}>
          <div className={styles.dialog} onClick={(e) => e.stopPropagation()}>
            <div className={styles.dialogHeader}>
              <h3>Agents in Pool</h3>
              <button className={styles.closeButton} onClick={() => setShowAgentsDialog(false)}>
                ×
              </button>
            </div>
            <div className={styles.dialogContent}>
              {poolAgents.length === 0 ? (
                <div className={styles.emptyState}>
                  <p>No agents in this pool</p>
                </div>
              ) : (
                <div className={styles.agentList}>
                  {poolAgents.map((agent) => (
                    <div key={agent.agent_id} className={styles.agentItem}>
                      <div className={styles.agentInfo}>
                        <div className={styles.agentName}>{agent.name}</div>
                        <div className={styles.agentId}>{agent.agent_id}</div>
                        <div className={styles.agentMeta}>
                          {getStatusBadge(agent.status)}
                          {agent.version && <span className={styles.version}>v{agent.version}</span>}
                          {agent.ip_address && <span className={styles.ip}>{agent.ip_address}</span>}
                        </div>
                        {agent.last_ping_at && (
                          <div className={styles.lastPing}>
                            Last ping: {new Date(agent.last_ping_at).toLocaleString()}
                          </div>
                        )}
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </div>
            <div className={styles.dialogFooter}>
              <button
                className={styles.closeDialogButton}
                onClick={() => setShowAgentsDialog(false)}
              >
                Close
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};

export default WorkspaceAgentConfig;
