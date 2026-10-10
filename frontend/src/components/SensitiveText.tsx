/**
 * 渲染可能含脱敏占位符 "(sensitive value)" 的值文本:占位符显示为灰色斜体 chip
 * (不加引号、无复制按钮),其余文本原样。
 */
import { splitSensitive, SENSITIVE_PLACEHOLDER } from '../utils/sensitiveValue';

const chipStyle: React.CSSProperties = {
  display: 'inline-block',
  padding: '0 6px',
  borderRadius: 3,
  background: 'rgba(128, 128, 128, 0.14)',
  color: 'var(--ink-faint, #8c8c8c)',
  fontStyle: 'italic',
  fontSize: '0.92em',
  lineHeight: 1.5,
  userSelect: 'none',
  whiteSpace: 'nowrap',
};

export function SensitiveChip() {
  return (
    <span style={chipStyle} title="该值已脱敏,不可查看">
      {SENSITIVE_PLACEHOLDER}
    </span>
  );
}

export default function SensitiveText({ text }: { text: string }) {
  const parts = splitSensitive(text);
  if (parts.length === 1 && parts[0] !== null) return <>{text}</>;
  return (
    <>
      {parts.map((p, i) => (p === null ? <SensitiveChip key={i} /> : <span key={i}>{p}</span>))}
    </>
  );
}
