/** 版本 bundle 状态标签(版本历史列表 / 部署面板):hash_mismatch 红色"完整性校验失败",其余橙色"需重新发布" */
import { Tag, Tooltip } from 'antd'
import type { ManifestVersion } from './manifestApi'
import { bundleStatusLabel, isHashMismatch } from './bundleStatus'

type BundleFields = Pick<ManifestVersion, 'bundle_hash' | 'bundle_invalid_reason'>

/** 合法版本不渲染 */
export default function BundleStatusTag({ version, style }: { version: BundleFields; style?: React.CSSProperties }) {
  const s = bundleStatusLabel(version)
  if (!s) return null
  return (
    <Tooltip title={<span style={{ whiteSpace: 'pre-line' }}>{s.tooltip}</span>}>
      <Tag
        color={isHashMismatch(version) ? 'error' : 'warning'}
        style={{ marginInlineEnd: 0, fontSize: 11, lineHeight: '16px', padding: '0 4px', ...style }}
      >
        {s.label}
      </Tag>
    </Tooltip>
  )
}

