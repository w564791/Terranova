/**
 * Manifest 审批绑定的哈希摘要(任务详情;后端 task detail 的 manifest_run,仅 manifest plan_and_apply 任务有)。
 *  - 审批前:"将审批：bundle <12> · plan <12>"(bundle_hash / plan_out_hash)
 *  - 审批后:"已审批：bundle <12> · plan <12> · <approved_by> · <approved_at>"(approved_*)
 *  - preview 运行(不可审批):"预览：bundle <12> · plan <12>"
 * 每个哈希只显示前 12 位,悬停看完整值,可复制。字段缺失时不渲染。
 * 结构与后端 services.TaskManifestRunView(e5ba55f)一致:未设置的值为 null。
 */
import { Typography } from 'antd';
import { parseBackendTime } from '../utils/time';

export interface TaskManifestRun {
  id?: string;
  /** preview | approval */
  purpose?: string;
  /** agent | sandbox */
  runner?: string;
  /** pending | running | succeeded | failed | cancelled */
  status?: string;
  bundle_hash?: string | null;
  /** SHA-256(plan.out),审批绑定的就是它 */
  plan_out_hash?: string | null;
  /** 脱敏 plan JSON 的哈希(展示内容) */
  redacted_plan_hash?: string | null;
  approved_bundle_hash?: string | null;
  approved_plan_hash?: string | null;
  approved_by?: string | null;
  approved_at?: string | null;
}

function Hash({ label, value }: { label: string; value: string }) {
  return (
    <span>
      {label}{' '}
      <Typography.Text
        code
        copyable={{ text: value, tooltips: ['复制完整哈希', '已复制'] }}
        title={value}
        style={{ fontSize: 12 }}
      >
        {value.slice(0, 12)}
      </Typography.Text>
    </span>
  );
}

function formatTime(s: string): string {
  try {
    return parseBackendTime(s).toLocaleString('zh-CN', { hour12: false });
  } catch {
    return s;
  }
}

export default function ManifestRunHashLine({ run }: { run?: TaskManifestRun | null }) {
  if (!run) return null;
  const approved = !!(run.approved_bundle_hash && run.approved_plan_hash);
  const bundle = approved ? run.approved_bundle_hash : run.bundle_hash;
  const plan = approved ? run.approved_plan_hash : run.plan_out_hash;
  if (!bundle && !plan) return null;
  return (
    <div style={{ fontSize: 12, color: 'var(--ink-faint, #8c8c8c)', margin: '4px 0 10px', display: 'flex', flexWrap: 'wrap', gap: 6, alignItems: 'center' }}>
      <span>{approved ? '已审批：' : run.purpose === 'preview' ? '预览：' : '将审批：'}</span>
      {bundle && <Hash label="bundle" value={bundle} />}
      {bundle && plan && <span>·</span>}
      {plan && <Hash label="plan" value={plan} />}
      {approved && run.approved_by && (
        <>
          <span>·</span>
          <span>{run.approved_by}</span>
        </>
      )}
      {approved && run.approved_at && (
        <>
          <span>·</span>
          <span>{formatTime(run.approved_at)}</span>
        </>
      )}
    </div>
  );
}
