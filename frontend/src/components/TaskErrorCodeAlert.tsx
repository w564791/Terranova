/**
 * 任务结构化失败码提示(任务详情页):按 error_code 显示红色 Alert + 原因;
 * bundle_republish_required 带"去升级"(打开 manifest 编辑器的部署面板并预选该 workspace);
 * plan_expired 带"重新 Plan"(复用任务页的 New run 对话框,由 onRerunPlan 提供,未提供则只显示文字)。
 * manifest id 取自 workspace 的 manifest 摘要(GET /workspaces/:id/manifest-summary);
 * 取不到(未绑定 / 已卸载 / 无权限)时退回 workspace 概览页(顶部有 manifest 摘要)。
 */
import { Alert, Button } from 'antd';
import { useNavigate } from 'react-router-dom';
import { taskErrorInfo } from '../utils/taskErrorCode';
import { useWorkspaceManifestSummary } from '../hooks/useWorkspaceManifestSummary';

interface Props {
  workspaceId: string;
  errorCode?: string | null;
  errorMessage?: string | null;
  /** 短原因 token(规则名 / 能力名),优先于解析 error_message */
  errorReason?: string | null;
  /** plan_expired 的"重新 Plan"入口(复用页面已有的新建运行) */
  onRerunPlan?: () => void;
}

export default function TaskErrorCodeAlert({ workspaceId, errorCode, errorMessage, errorReason, onRerunPlan }: Props) {
  const navigate = useNavigate();
  const info = taskErrorInfo(errorCode, errorMessage, errorReason);
  // 只有需要升级入口时才拉 manifest 摘要
  const { summary, loading } = useWorkspaceManifestSummary(
    info?.action === 'upgrade_manifest' ? workspaceId : undefined,
  );
  if (!info) return null;

  let upgradeTarget: string | null = null;
  if (info.action === 'upgrade_manifest') {
    if (summary?.has_manifest && summary.manifest_id && summary.org_id != null) {
      const params = new URLSearchParams({ org: String(summary.org_id), deploy: workspaceId });
      upgradeTarget = `/admin/manifests-v2/${summary.manifest_id}/edit?${params.toString()}`;
    } else {
      upgradeTarget = `/workspaces/${workspaceId}`;
    }
  }

  return (
    <Alert
      type="error"
      showIcon
      style={{ marginBottom: 12 }}
      message={info.title}
      description={
        info.detail ? (
          <span style={{ whiteSpace: 'pre-line' }} title={info.rawDetail !== info.detail ? info.rawDetail : undefined}>
            {info.detail}
          </span>
        ) : undefined
      }
      action={
        upgradeTarget ? (
          <Button size="small" type="primary" danger loading={loading} onClick={() => navigate(upgradeTarget!)}>
            去升级
          </Button>
        ) : info.action === 'rerun_plan' && onRerunPlan ? (
          <Button size="small" type="primary" onClick={onRerunPlan}>
            重新 Plan
          </Button>
        ) : undefined
      }
    />
  );
}
