import React, { useCallback, useEffect, useState } from 'react';
import { useNavigate, useParams, useSearchParams } from 'react-router-dom';
import { setAuthOrgId, getAuthOrgId } from '../../services/iam';
import {
  sandboxesApi,
  isPublicNetworkMode,
  isUnknownNetworkMode,
  maskExternalId,
  type Sandbox,
  type SandboxDriver,
  type SandboxStatus,
  type ShellSession,
} from '../../services/sandboxes';
import { useToast } from '../../contexts/ToastContext';
import { extractErrorMessageWithCode } from '../../utils/errorHandler';
import ConfirmDialog from '../../components/ConfirmDialog';
import styles from './SandboxDetail.module.css';

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
  return { label: String(mode), className: styles.networkUnknown };
}

const SandboxDetail: React.FC = () => {
  const { sandboxId } = useParams<{ sandboxId: string }>();
  const [searchParams] = useSearchParams();
  const navigate = useNavigate();
  const { showToast } = useToast();

  const orgFromQuery = Number(searchParams.get('org_id'));
  const orgId =
    Number.isFinite(orgFromQuery) && orgFromQuery > 0
      ? orgFromQuery
      : getAuthOrgId();

  const [sandbox, setSandbox] = useState<Sandbox | null>(null);
  const [loading, setLoading] = useState(true);
  const [unbindOpen, setUnbindOpen] = useState(false);
  const [unbinding, setUnbinding] = useState(false);
  const [shellSession, setShellSession] = useState<ShellSession | null>(null);
  /** typ=shell JWT — memory only; never localStorage / console. */
  const [shellToken, setShellToken] = useState<string | null>(null);
  const [tokenExpiresAt, setTokenExpiresAt] = useState<string | null>(null);
  const [shellBusy, setShellBusy] = useState(false);

  const load = useCallback(async () => {
    if (!sandboxId || !orgId) {
      setLoading(false);
      return;
    }
    try {
      setLoading(true);
      setAuthOrgId(orgId);
      const row = await sandboxesApi.get(orgId, sandboxId);
      setSandbox(row);
    } catch (error) {
      showToast(extractErrorMessageWithCode(error), 'error');
      navigate('/global/settings/sandboxes');
    } finally {
      setLoading(false);
    }
  }, [sandboxId, orgId, navigate, showToast]);

  useEffect(() => {
    load();
  }, [load]);

  const handleUnbind = async () => {
    if (!sandbox || !orgId) return;
    try {
      setUnbinding(true);
      await sandboxesApi.unbind(orgId, sandbox.id);
      showToast('已解绑登记（远端 harness 未销毁）', 'success');
      setUnbindOpen(false);
      navigate('/global/settings/sandboxes');
    } catch (error) {
      showToast(extractErrorMessageWithCode(error), 'error');
    } finally {
      setUnbinding(false);
    }
  };

  const openShell = async () => {
    if (!sandbox || !orgId || sandbox.driver !== 'shell_debug') return;
    try {
      setShellBusy(true);
      const res = await sandboxesApi.createShellSession(orgId, sandbox.id);
      const { shell_token, token_expires_at, ...sess } = res;
      setShellSession(sess);
      // One-shot memory hold — do not log or persist the raw token.
      setShellToken(typeof shell_token === 'string' && shell_token ? shell_token : null);
      setTokenExpiresAt(
        typeof token_expires_at === 'string' ? token_expires_at : sess.expires_at || null,
      );
      showToast(
        shell_token ? 'Shell 会话已打开（凭证仅存内存）' : 'Shell 会话已打开',
        'success',
      );
    } catch (error) {
      setShellToken(null);
      setTokenExpiresAt(null);
      showToast(extractErrorMessageWithCode(error), 'error');
    } finally {
      setShellBusy(false);
    }
  };

  const closeShell = async () => {
    if (!orgId || !shellSession) return;
    try {
      setShellBusy(true);
      await sandboxesApi.closeShellSession(orgId, shellSession.id);
      setShellSession({ ...shellSession, status: 'closed', closed_at: new Date().toISOString() });
      setShellToken(null);
      setTokenExpiresAt(null);
      showToast('Shell 会话已关闭并撤销凭证', 'success');
    } catch (error) {
      showToast(extractErrorMessageWithCode(error), 'error');
    } finally {
      setShellBusy(false);
    }
  };

  if (loading) {
    return <div className={styles.loading}>加载沙箱详情…</div>;
  }
  if (!sandbox || !orgId) {
    return <div className={styles.loading}>未找到沙箱或未选择组织</div>;
  }

  const net = networkBadge(sandbox.network_mode);
  const isShell = sandbox.driver === 'shell_debug';
  const showPublicWarn = isPublicNetworkMode(sandbox.network_mode);
  const showUnknownWarn = isUnknownNetworkMode(sandbox.network_mode);
  const title = sandbox.label?.trim() || sandbox.sandbox_ref;

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <button
          type="button"
          className={styles.backButton}
          onClick={() => navigate('/global/settings/sandboxes')}
        >
          ← 返回列表
        </button>
        <div className={styles.titleRow}>
          <div className={styles.titleBlock}>
            <h1 title={sandbox.sandbox_ref}>{title}</h1>
            <div className={styles.chips}>
              <span
                className={`${styles.chip} ${
                  isShell ? styles.driverShell : styles.driverAgentcore
                }`}
              >
                {DRIVER_LABELS[sandbox.driver as SandboxDriver] || sandbox.driver}
              </span>
              <span className={`${styles.chip} ${statusClass(sandbox.status)}`}>
                {STATUS_LABELS[sandbox.status as SandboxStatus] || sandbox.status}
              </span>
              <span className={`${styles.chip} ${net.className}`}>{net.label}</span>
            </div>
          </div>
          {sandbox.status !== 'unbound' && (
            <button
              type="button"
              className={styles.unbindButton}
              onClick={() => setUnbindOpen(true)}
            >
              解绑登记
            </button>
          )}
        </div>
      </div>

      {showPublicWarn && (
        <div className={styles.warningBanner} role="alert">
          <span className={styles.warningIcon}>⚠</span>
          <span>
            网络模式为公网（PUBLIC / 联调）。生产环境 Deploy / 绑定将被拒绝（API：network_mode_not_allowed）。请改用 VPC 或换非生产环境。
          </span>
        </div>
      )}
      {showUnknownWarn && (
        <div className={styles.warningBanner} role="alert">
          <span className={styles.warningIcon}>⚠</span>
          <span>
            未记录 network_mode 快照（未知）。生产环境 Deploy / 唤醒 / 绑定时将按
            network_mode_not_allowed 拒绝，直至快照为 VPC。
          </span>
        </div>
      )}

      <div className={styles.card}>
        <h2 className={styles.cardTitle}>登记信息</h2>
        <dl className={styles.dl}>
          <dt>sandbox_ref</dt>
          <dd className={styles.mono}>{sandbox.sandbox_ref}</dd>
          <dt>runtime_ref（暖池）</dt>
          <dd className={styles.mono}>{sandbox.runtime_ref || '—'}</dd>
          <dt>工作空间</dt>
          <dd className={styles.mono}>{sandbox.workspace_id}</dd>
          <dt>image_digest</dt>
          <dd className={styles.mono}>{sandbox.image_digest || '—'}</dd>
          <dt>assume_role_arn</dt>
          <dd className={styles.mono}>{sandbox.assume_role_arn || '—'}</dd>
          <dt>assume_external_id</dt>
          <dd className={styles.mono} title="已遮罩，防 confused-deputy 泄露">
            {maskExternalId(sandbox.assume_external_id)}
          </dd>
          <dt>租约到期</dt>
          <dd>
            {sandbox.lease_expires_at
              ? new Date(sandbox.lease_expires_at).toLocaleString()
              : '—'}
          </dd>
          <dt>创建者</dt>
          <dd>{sandbox.created_by}</dd>
          <dt>登记时间</dt>
          <dd>{new Date(sandbox.created_at).toLocaleString()}</dd>
          <dt>更新时间</dt>
          <dd>{new Date(sandbox.updated_at).toLocaleString()}</dd>
        </dl>
        <div className={styles.runtimeNote}>
          runtime/harness 常驻；新会话只唤醒，不冷启整池。会话状态与暖池 runtime_ref
          分开理解；唤醒仅走 InvokeHarness。
        </div>
      </div>

      {isShell ? (
        <div className={styles.card}>
          <h2 className={styles.cardTitle}>Shell 会话</h2>
          <p className={styles.shellHint}>
            仅 shell_debug 驱动支持。打开会话：AssertAttachable → InvokeHarness 唤醒 → 签发 typ=shell（≤15min）。凭证只存浏览器内存，关闭即撤销；不进 localStorage / 控制台。不会销毁远端容器。
          </p>
          <div className={styles.shellActions}>
            <button
              type="button"
              className={styles.primaryButton}
              onClick={openShell}
              disabled={shellBusy || sandbox.status === 'unbound'}
            >
              {shellBusy ? '处理中…' : '打开 Shell 会话'}
            </button>
            <button
              type="button"
              className={styles.secondaryButton}
              onClick={closeShell}
              disabled={
                shellBusy ||
                !shellSession ||
                shellSession.status !== 'active' ||
                !!shellSession.closed_at
              }
            >
              关闭会话
            </button>
          </div>
          {shellSession && (
            <div className={styles.sessionBox}>
              <dl className={styles.sessionMeta}>
                <dt>session_id</dt>
                <dd className={styles.mono}>{shellSession.id}</dd>
                <dt>状态</dt>
                <dd>{shellSession.status}</dd>
                <dt>过期时间</dt>
                <dd>{new Date(shellSession.expires_at).toLocaleString()}</dd>
                <dt>关闭时间</dt>
                <dd>
                  {shellSession.closed_at
                    ? new Date(shellSession.closed_at).toLocaleString()
                    : '—'}
                </dd>
                <dt>短时凭证</dt>
                <dd>
                  {shellSession.status === 'active' && shellToken
                    ? `已持有（仅内存，不展示明文）${
                        tokenExpiresAt
                          ? ` · 过期 ${new Date(tokenExpiresAt).toLocaleString()}`
                          : ''
                      }`
                    : shellSession.status === 'active'
                      ? '本次响应无 shell_token'
                      : '已清除'}
                </dd>
              </dl>
            </div>
          )}
        </div>
      ) : (
        <div className={styles.card}>
          <h2 className={styles.cardTitle}>AgentCore 外部</h2>
          <p className={styles.agentcoreNote}>
            此驱动仅作外部登记与 Deploy 绑定准备。Shell 会话不可用（API：shell_not_supported）。
            远端 harness 生命周期由云侧管理；此处「解绑登记」不会销毁远端。
          </p>
        </div>
      )}

      <ConfirmDialog
        isOpen={unbindOpen}
        title="解绑沙箱登记？"
        message="仅移除平台登记并撤销未过期凭证钩子。远端 AgentCore harness / runtime 不会被销毁。此操作不可在列表中撤销（状态变为 unbound）。"
        confirmText="解绑登记"
        cancelText="取消"
        type="danger"
        loading={unbinding}
        onConfirm={handleUnbind}
        onCancel={() => setUnbindOpen(false)}
      />
    </div>
  );
};

export default SandboxDetail;
