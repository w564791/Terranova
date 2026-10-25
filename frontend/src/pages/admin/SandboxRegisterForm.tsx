import React, { useEffect, useState } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { iamService, setAuthOrgId } from '../../services/iam';
import { workspaceService, type Workspace } from '../../services/workspaces';
import {
  sandboxesApi,
  isValidSandboxRef,
  isValidOptionalRuntimeRef,
  type SandboxDriver,
  type SandboxNetworkMode,
} from '../../services/sandboxes';
import { useToast } from '../../contexts/ToastContext';
import { extractErrorMessageWithCode } from '../../utils/errorHandler';
import styles from './SandboxRegisterForm.module.css';

interface Organization {
  id: number;
  name: string;
  display_name?: string;
}

function workspaceKey(ws: Workspace): string {
  return ws.workspace_id || String(ws.id);
}

const SandboxRegisterForm: React.FC = () => {
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const { showToast } = useToast();

  const [organizations, setOrganizations] = useState<Organization[]>([]);
  const [orgId, setOrgId] = useState<number | null>(null);
  const [workspaces, setWorkspaces] = useState<Workspace[]>([]);
  const [loadingMeta, setLoadingMeta] = useState(true);
  const [submitting, setSubmitting] = useState(false);

  const [driver, setDriver] = useState<SandboxDriver>('agentcore_external');
  const [workspaceId, setWorkspaceId] = useState('');
  const [sandboxRef, setSandboxRef] = useState('');
  const [runtimeRef, setRuntimeRef] = useState('');
  const [label, setLabel] = useState('');
  const [imageDigest, setImageDigest] = useState('');
  const [networkMode, setNetworkMode] = useState<'' | SandboxNetworkMode>('');
  const [assumeRoleArn, setAssumeRoleArn] = useState('');
  const [assumeExternalId, setAssumeExternalId] = useState('');

  const [refError, setRefError] = useState<string | null>(null);
  const [runtimeError, setRuntimeError] = useState<string | null>(null);

  useEffect(() => {
    const boot = async () => {
      try {
        setLoadingMeta(true);
        const response = await iamService.bootstrapActiveOrganization();
        const orgs = response.organizations || [];
        setOrganizations(orgs);
        const fromQuery = Number(searchParams.get('org_id'));
        const initial =
          Number.isFinite(fromQuery) && fromQuery > 0
            ? fromQuery
            : response.active_org_id ?? orgs[0]?.id ?? null;
        if (initial != null) {
          setAuthOrgId(initial);
          setOrgId(initial);
        }
      } catch (error) {
        showToast(extractErrorMessageWithCode(error), 'error');
      } finally {
        setLoadingMeta(false);
      }
    };
    boot();
  }, [searchParams, showToast]);

  useEffect(() => {
    if (!orgId) {
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
        try {
          const res = await workspaceService.getWorkspaces();
          setWorkspaces(Array.isArray(res?.data) ? res.data : []);
        } catch (error) {
          showToast(extractErrorMessageWithCode(error), 'error');
          setWorkspaces([]);
        }
      }
    };
    loadWs();
  }, [orgId, showToast]);

  const validateRefs = (): boolean => {
    let ok = true;
    if (!isValidSandboxRef(sandboxRef)) {
      setRefError(
        sandboxRef.trim().includes('://')
          ? 'sandbox_ref 不能是 URL（禁止包含 ://）'
          : '请填写 sandbox_ref（ARN 或短 ID，不可为空）',
      );
      ok = false;
    } else {
      setRefError(null);
    }
    if (!isValidOptionalRuntimeRef(runtimeRef)) {
      setRuntimeError(
        runtimeRef.trim().includes('://')
          ? 'runtime_ref 不能是 URL（禁止包含 ://）'
          : 'runtime_ref 格式不合法',
      );
      ok = false;
    } else {
      setRuntimeError(null);
    }
    return ok;
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!orgId) {
      showToast('请先选择组织', 'error');
      return;
    }
    if (!workspaceId) {
      showToast('请选择工作空间', 'error');
      return;
    }
    if (!validateRefs()) return;

    const body: Parameters<typeof sandboxesApi.register>[1] = {
      workspace_id: workspaceId,
      sandbox_ref: sandboxRef.trim(),
      driver,
    };
    if (label.trim()) body.label = label.trim();
    if (runtimeRef.trim()) body.runtime_ref = runtimeRef.trim();
    if (imageDigest.trim()) body.image_digest = imageDigest.trim();
    if (networkMode) body.network_mode = networkMode;
    if (assumeRoleArn.trim()) body.assume_role_arn = assumeRoleArn.trim();
    if (assumeExternalId.trim()) body.assume_external_id = assumeExternalId.trim();

    try {
      setSubmitting(true);
      const created = await sandboxesApi.register(orgId, body);
      showToast('沙箱已登记（仅平台登记，未创建远端资源）', 'success');
      navigate(`/global/settings/sandboxes/${created.id}?org_id=${orgId}`);
    } catch (error) {
      showToast(extractErrorMessageWithCode(error), 'error');
    } finally {
      setSubmitting(false);
    }
  };

  if (loadingMeta) {
    return <div className={styles.loading}>加载中…</div>;
  }

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <button type="button" className={styles.backButton} onClick={() => navigate(-1)}>
          ← 返回
        </button>
        <h1>登记外部沙箱</h1>
        <p className={styles.lead}>
          平台只登记已存在的 AgentCore harness / runtime 引用，不会在云侧创建或销毁容器。
          生产 Deploy / 绑定要求 network_mode=VPC；公网仅用于联调。
        </p>
      </div>

      <form className={styles.form} onSubmit={handleSubmit} noValidate>
        <div className={styles.section}>
          <h2 className={styles.sectionTitle}>组织与驱动</h2>
          <div className={styles.formGroup}>
            <label htmlFor="org">组织 *</label>
            <select
              id="org"
              value={orgId ?? ''}
              onChange={(e) => {
                const v = Number(e.target.value);
                setAuthOrgId(v);
                setOrgId(v);
                setWorkspaceId('');
              }}
              required
            >
              {organizations.map((org) => (
                <option key={org.id} value={org.id}>
                  {org.display_name || org.name}
                </option>
              ))}
            </select>
          </div>

          <div className={styles.formGroup}>
            <label>驱动 *</label>
            <div className={styles.driverGrid}>
              <button
                type="button"
                className={`${styles.driverCard} ${driver === 'agentcore_external' ? styles.driverCardActive : ''}`}
                onClick={() => setDriver('agentcore_external')}
              >
                <span className={styles.driverCardTitle}>AgentCore 外部登记</span>
                <span className={styles.driverCardDesc}>
                  登记已有 harness/runtime。无 Shell 会话；唤醒走 InvokeHarness。
                </span>
              </button>
              <button
                type="button"
                className={`${styles.driverCard} ${driver === 'shell_debug' ? styles.driverCardActive : ''}`}
                onClick={() => setDriver('shell_debug')}
              >
                <span className={styles.driverCardTitle}>Shell 调试</span>
                <span className={styles.driverCardDesc}>
                  仅联调/排障。可在详情页开关短时 Shell 会话；与 AgentCore 生产路径分离。
                </span>
              </button>
            </div>
          </div>
        </div>

        <div className={styles.section}>
          <h2 className={styles.sectionTitle}>绑定与引用</h2>
          <div className={styles.formGroup}>
            <label htmlFor="workspace">工作空间 *</label>
            <select
              id="workspace"
              value={workspaceId}
              onChange={(e) => setWorkspaceId(e.target.value)}
              required
            >
              <option value="">选择工作空间…</option>
              {workspaces.map((ws) => (
                <option key={workspaceKey(ws)} value={workspaceKey(ws)}>
                  {ws.name} ({workspaceKey(ws)})
                </option>
              ))}
            </select>
            <span className={styles.helpText}>需要该工作空间的 WORKSPACE_RESOURCES WRITE 权限。</span>
          </div>

          <div className={styles.formGroup}>
            <label htmlFor="sandbox_ref">sandbox_ref *</label>
            <input
              id="sandbox_ref"
              type="text"
              value={sandboxRef}
              onChange={(e) => {
                setSandboxRef(e.target.value);
                setRefError(null);
              }}
              onBlur={validateRefs}
              className={refError ? styles.invalid : undefined}
              placeholder="arn:aws:bedrock-agentcore:…:harness/… 或 harness_…"
              autoComplete="off"
              required
            />
            {refError ? (
              <span className={styles.fieldError}>{refError}</span>
            ) : (
              <span className={styles.helpText}>
                支持完整 ARN（可含 /）或短 ID；禁止 URL（://）。不在此创建远端 harness。
              </span>
            )}
          </div>

          <div className={styles.runtimeNote}>
            runtime/harness 常驻；新会话只唤醒，不冷启整池。InvokeHarness 唤醒，勿与
            session 状态混淆。
          </div>

          <div className={styles.formGroup}>
            <label htmlFor="runtime_ref">runtime_ref（暖池，可选）</label>
            <input
              id="runtime_ref"
              type="text"
              value={runtimeRef}
              onChange={(e) => {
                setRuntimeRef(e.target.value);
                setRuntimeError(null);
              }}
              onBlur={validateRefs}
              className={runtimeError ? styles.invalid : undefined}
              placeholder="arn:aws:bedrock-agentcore:…:runtime/… 或短 ID"
              autoComplete="off"
            />
            {runtimeError ? (
              <span className={styles.fieldError}>{runtimeError}</span>
            ) : (
              <span className={styles.helpText}>与会话/唤醒状态分开展示；可空。</span>
            )}
          </div>

          <div className={styles.formGroup}>
            <label htmlFor="label">标签（可选）</label>
            <input
              id="label"
              type="text"
              value={label}
              onChange={(e) => setLabel(e.target.value)}
              placeholder="例如：联调 / 生产 VPC"
              maxLength={256}
            />
          </div>

          <div className={styles.formGroup}>
            <label htmlFor="image_digest">image_digest（可选）</label>
            <input
              id="image_digest"
              type="text"
              value={imageDigest}
              onChange={(e) => setImageDigest(e.target.value)}
              placeholder="sha256:…"
              autoComplete="off"
            />
          </div>
        </div>

        <div className={styles.section}>
          <h2 className={styles.sectionTitle}>网络模式</h2>
          <div className={styles.formGroup}>
            <label htmlFor="network_mode">network_mode（可选，建议生产选 VPC）</label>
            <select
              id="network_mode"
              value={networkMode}
              onChange={(e) => setNetworkMode(e.target.value as '' | SandboxNetworkMode)}
            >
              <option value="">未指定（快照未知）</option>
              <option value="VPC">VPC（生产 Deploy 所需）</option>
              <option value="PUBLIC">PUBLIC（公网 / 联调）</option>
            </select>
            <span className={styles.helpText}>
              生产环境 attach / Deploy 仅接受 VPC。未指定或 PUBLIC 在生产会被拒绝（network_mode_not_allowed）。无客户端「允许公网」开关。
            </span>
          </div>
          {networkMode === 'PUBLIC' && (
            <div className={styles.warningBanner} role="alert">
              <span className={styles.warningIcon}>⚠</span>
              <span>
                已选择公网（PUBLIC / 联调）。生产环境 Deploy / 绑定将被拒绝（API：network_mode_not_allowed）。请改用 VPC，或仅在非生产环境联调。
              </span>
            </div>
          )}
          {networkMode === '' && (
            <div className={styles.warningBanner} role="status">
              <span className={styles.warningIcon}>ℹ</span>
              <span>
                未指定 network_mode 时快照为「未知」。若该登记日后用于生产 Deploy，平台将按
                network_mode_not_allowed 拒绝，直至快照为 VPC。
              </span>
            </div>
          )}
        </div>

        <div className={styles.section}>
          <h2 className={styles.sectionTitle}>可选 AssumeRole（明文，无密钥）</h2>
          <div className={styles.formGroup}>
            <label htmlFor="assume_role_arn">assume_role_arn</label>
            <input
              id="assume_role_arn"
              type="text"
              value={assumeRoleArn}
              onChange={(e) => setAssumeRoleArn(e.target.value)}
              placeholder="arn:aws:iam::…:role/…"
              autoComplete="off"
            />
            <span className={styles.helpText}>不展示或索取 AWS Access Key；平台凭证在 Vault。</span>
          </div>
          <div className={styles.formGroup}>
            <label htmlFor="assume_external_id">assume_external_id</label>
            <input
              id="assume_external_id"
              type="text"
              value={assumeExternalId}
              onChange={(e) => setAssumeExternalId(e.target.value)}
              autoComplete="off"
            />
          </div>
        </div>

        <div className={styles.formActions}>
          <button
            type="button"
            className={styles.cancelButton}
            onClick={() => navigate('/global/settings/sandboxes')}
            disabled={submitting}
          >
            取消
          </button>
          <button type="submit" className={styles.submitButton} disabled={submitting || !orgId}>
            {submitting ? '登记中…' : '登记（不解绑远端）'}
          </button>
        </div>
      </form>
    </div>
  );
};

export default SandboxRegisterForm;
