import React, { useCallback, useEffect, useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { iamService, setAuthOrgId } from '../../services/iam';
import { workspaceService, type Workspace } from '../../services/workspaces';
import {
  sandboxesApi,
  isPublicNetworkMode,
  isUnknownNetworkMode,
  type Sandbox,
  type SandboxDriver,
  type SandboxStatus,
} from '../../services/sandboxes';
import { useToast } from '../../contexts/ToastContext';
import { extractErrorMessageWithCode } from '../../utils/errorHandler';
import styles from './SandboxManagement.module.css';

interface Organization {
  id: number;
  name: string;
  display_name?: string;
}

const STATUS_LABELS: Record<SandboxStatus, string> = {
  registered: '已登记',
  ready: '就绪',
  lease_expired: '租约过期',
  unbound: '已解绑',
};

const DRIVER_LABELS: Record<SandboxDriver, string> = {
  agentcore_external: 'AgentCore 外部',
  shell_debug: 'Shell 调试',
};

function statusClass(status: string): string {
  switch (status) {
    case 'ready':
      return styles.statusReady;
    case 'registered':
      return styles.statusRegistered;
    case 'lease_expired':
      return styles.statusLeaseExpired;
    default:
      return styles.statusDefault;
  }
}

function driverClass(driver: string): string {
  return driver === 'shell_debug' ? styles.driverShell : styles.driverAgentcore;
}

function networkBadge(mode?: string | null): { label: string; className: string } {
  if (isUnknownNetworkMode(mode)) {
    return { label: '未知', className: styles.networkUnknown };
  }
  if (isPublicNetworkMode(mode)) {
    return { label: '公网 / 联调', className: styles.networkPublic };
  }
  if ((mode || '').toUpperCase() === 'VPC') {
    return { label: 'VPC', className: styles.networkVpc };
  }
  return { label: mode || '未知', className: styles.networkUnknown };
}

function workspaceKey(ws: Workspace): string {
  return ws.workspace_id || String(ws.id);
}

const SandboxManagement: React.FC = () => {
  const navigate = useNavigate();
  const { showToast } = useToast();
  const [organizations, setOrganizations] = useState<Organization[]>([]);
  const [selectedOrgId, setSelectedOrgId] = useState<number | null>(null);
  const [workspaces, setWorkspaces] = useState<Workspace[]>([]);
  const [workspaceFilter, setWorkspaceFilter] = useState('');
  const [items, setItems] = useState<Sandbox[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    const loadOrgs = async () => {
      try {
        const response = await iamService.bootstrapActiveOrganization();
        setOrganizations(response.organizations || []);
        if (response.active_org_id != null) {
          setSelectedOrgId(response.active_org_id);
        }
      } catch (error) {
        console.error('加载组织列表失败:', error);
        showToast(extractErrorMessageWithCode(error), 'error');
      }
    };
    loadOrgs();
  }, [showToast]);

  useEffect(() => {
    if (!selectedOrgId) {
      setWorkspaces([]);
      return;
    }
    const loadWs = async () => {
      try {
        const res = await workspaceService.getWorkspaces({
          capability: 'WORKSPACE_RESOURCES:WRITE',
        });
        setWorkspaces(Array.isArray(res?.data) ? res.data : []);
      } catch {
        // Fall back to unfiltered list if capability filter unavailable
        try {
          const res = await workspaceService.getWorkspaces();
          setWorkspaces(Array.isArray(res?.data) ? res.data : []);
        } catch (error) {
          console.error('加载工作空间失败:', error);
          setWorkspaces([]);
        }
      }
    };
    loadWs();
  }, [selectedOrgId]);

  const loadSandboxes = useCallback(async () => {
    if (!selectedOrgId) {
      setItems([]);
      setTotal(0);
      setLoading(false);
      return;
    }
    try {
      setLoading(true);
      const params = workspaceFilter ? { workspace_id: workspaceFilter } : undefined;
      const res = await sandboxesApi.list(selectedOrgId, params);
      setItems(res.items);
      setTotal(res.total);
    } catch (error) {
      showToast(extractErrorMessageWithCode(error), 'error');
      setItems([]);
      setTotal(0);
    } finally {
      setLoading(false);
    }
  }, [selectedOrgId, workspaceFilter, showToast]);

  useEffect(() => {
    loadSandboxes();
  }, [loadSandboxes]);

  const onOrgChange = (value: number) => {
    setAuthOrgId(value);
    setSelectedOrgId(value);
    setWorkspaceFilter('');
  };

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <div className={styles.headerLeft}>
          <h1>沙箱登记</h1>
          <p className={styles.subtitle}>
            仅登记平台外已创建的 AgentCore harness / runtime（或 Shell 调试驱动）。
            解绑不会销毁远端资源。runtime/harness 常驻；新会话只唤醒，不冷启整池。
          </p>
        </div>
        <div className={styles.headerActions}>
          {organizations.length > 0 && (
            <select
              className={styles.orgSelect}
              value={selectedOrgId ?? ''}
              onChange={(e) => onOrgChange(Number(e.target.value))}
              aria-label="组织"
            >
              {organizations.map((org) => (
                <option key={org.id} value={org.id}>
                  {org.display_name || org.name}
                </option>
              ))}
            </select>
          )}
          <button
            type="button"
            className={styles.createButton}
            disabled={!selectedOrgId}
            onClick={() =>
              navigate(
                `/global/settings/sandboxes/register${selectedOrgId ? `?org_id=${selectedOrgId}` : ''}`,
              )
            }
          >
            + 登记沙箱
          </button>
        </div>
      </div>

      <div className={styles.toolbar}>
        <label htmlFor="sandbox-ws-filter">工作空间</label>
        <select
          id="sandbox-ws-filter"
          className={styles.workspaceFilter}
          value={workspaceFilter}
          onChange={(e) => setWorkspaceFilter(e.target.value)}
          disabled={!selectedOrgId}
        >
          <option value="">全部</option>
          {workspaces.map((ws) => (
            <option key={workspaceKey(ws)} value={workspaceKey(ws)}>
              {ws.name} ({workspaceKey(ws)})
            </option>
          ))}
        </select>
      </div>

      <div className={styles.listCard}>
        {loading ? (
          <div className={styles.loading}>加载沙箱登记…</div>
        ) : items.length === 0 ? (
          <div className={styles.emptyState}>
            <p>暂无已登记沙箱</p>
            <p className={styles.emptyHint}>
              请先在 AWS / AgentCore 侧创建 harness，再在此登记引用（不会在平台内创建远端容器）。
            </p>
            <button
              type="button"
              disabled={!selectedOrgId}
              onClick={() =>
                navigate(
                  `/global/settings/sandboxes/register${selectedOrgId ? `?org_id=${selectedOrgId}` : ''}`,
                )
              }
            >
              登记第一个沙箱
            </button>
          </div>
        ) : (
          <>
            <table className={styles.table}>
              <thead>
                <tr>
                  <th>标签 / Ref</th>
                  <th>驱动</th>
                  <th>网络</th>
                  <th>状态</th>
                  <th>工作空间</th>
                  <th>Runtime</th>
                  <th>登记时间</th>
                </tr>
              </thead>
              <tbody>
                {items.map((row) => {
                  const net = networkBadge(row.network_mode);
                  const title = row.label?.trim() || row.sandbox_ref;
                  return (
                    <tr
                      key={row.id}
                      className={styles.clickableRow}
                      onClick={() =>
                        navigate(
                          `/global/settings/sandboxes/${row.id}?org_id=${row.org_id}`,
                        )
                      }
                    >
                      <td>
                        <div className={styles.labelCell}>{title}</div>
                        <div className={styles.refCell}>
                          <Link
                            to={`/global/settings/sandboxes/${row.id}?org_id=${row.org_id}`}
                            className={styles.refLink}
                            onClick={(e) => e.stopPropagation()}
                          >
                            {row.sandbox_ref}
                          </Link>
                        </div>
                      </td>
                      <td>
                        <span className={`${styles.chip} ${driverClass(row.driver)}`}>
                          {DRIVER_LABELS[row.driver as SandboxDriver] || row.driver}
                        </span>
                      </td>
                      <td>
                        <span className={`${styles.chip} ${net.className}`}>{net.label}</span>
                      </td>
                      <td>
                        <span className={`${styles.chip} ${statusClass(row.status)}`}>
                          {STATUS_LABELS[row.status as SandboxStatus] || row.status}
                        </span>
                      </td>
                      <td className={styles.mono}>{row.workspace_id}</td>
                      <td className={styles.mono}>{row.runtime_ref || '—'}</td>
                      <td>{new Date(row.created_at).toLocaleString()}</td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
            <div className={styles.metaRow}>
              <span>共 {total} 条（不含已解绑）</span>
            </div>
          </>
        )}
      </div>
    </div>
  );
};

export default SandboxManagement;
