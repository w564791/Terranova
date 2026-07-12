import React, { useState, useMemo } from 'react';
import styles from './PlanCompleteView.module.css';
import api from '../services/api';
import { softBadgeColors } from '../utils/contrast';
import { colors } from '../styles/tokens';

/** Lifecycle trigger event → soft badge with auto contrast */
const triggerEventStyle = (event: string): React.CSSProperties => {
  const base =
    event === 'AfterCreate'
      ? colors.green
      : event === 'AfterUpdate'
        ? colors.brand
        : event === 'BeforeDestroy'
          ? colors.red
          : colors.amber;
  return softBadgeColors(base);
};

const formatTriggerEvent = (event: string) => {
  switch (event) {
    case 'AfterCreate':
      return 'After Create';
    case 'AfterUpdate':
      return 'After Update';
    case 'BeforeDestroy':
      return 'Before Destroy';
    default:
      return event;
  }
};

interface ResourceChange {
  id: number;
  resource_address: string;
  resource_type: string;
  resource_name: string;
  module_address: string;
  action: string;
  changes_before: Record<string, any>;
  changes_after: Record<string, any>;
  after_unknown: Record<string, any>;
  apply_status: string;
}

interface OutputChange {
  name: string;
  action: string;
  before: any;
  after: any;
  after_unknown: boolean;
  sensitive: boolean;
}

interface ActionInvocation {
  name: string;
  type: string;
  address: string;
  config_values?: Record<string, any>;
  config_unknown?: Record<string, any>;
  provider_name?: string;
  lifecycle_action_trigger?: {
    actions_list_index: number;
    action_trigger_event: string;
    action_trigger_block_index: number;
    triggering_resource_address: string;
  };
}

// Action 资源定义（来自 configuration.root_module.module_calls.*.module.actions）
interface ActionResource {
  name: string;
  type: string;
  address: string;
  full_address?: string;
  module_address?: string;
  provider_config_key?: string;
}

interface Props {
  resources: ResourceChange[];
  summary: {
    add: number;
    change: number;
    destroy: number;
  };
  outputChanges?: OutputChange[];
  actionInvocations?: ActionInvocation[];
  actions?: ActionResource[];
  workspaceId?: string;
  taskId?: number;
}

// 判断值是否为空或无意义
const isEmptyValue = (value: any): boolean => {
  if (value === null || value === undefined) return true;
  if (value === '') return true;
  if (Array.isArray(value) && value.length === 0) return true;
  if (typeof value === 'object' && !Array.isArray(value) && Object.keys(value).length === 0) return true;
  return false;
};

// 格式化简单值
const formatSimpleValue = (value: any): string => {
  if (value === null) return 'null';
  if (value === undefined) return 'undefined';
  if (value === '') return '""';
  if (typeof value === 'boolean') return value ? 'true' : 'false';
  if (typeof value === 'number') return String(value);
  if (typeof value === 'string') return `"${value}"`;
  if (Array.isArray(value)) return JSON.stringify(value);
  if (typeof value === 'object') return JSON.stringify(value);
  return String(value);
};

// HCL 对象/数组序列化:JSON 值转 HCL 字面量风格(键去引号、= 代替 :、无逗号)。
// 键名不是合法 HCL 标识符时(如 "aws:SecureTransport")保留引号。
const hclKey = (k: string): string => /^[A-Za-z_][A-Za-z0-9_-]*$/.test(k) ? k : JSON.stringify(k);
const hclIndent = (depth: number): string => '  '.repeat(depth);
const toHcl = (value: any, depth = 0): string => {
  if (value === null || value === undefined) return 'null';
  if (typeof value === 'boolean') return value ? 'true' : 'false';
  if (typeof value === 'number') return String(value);
  if (typeof value === 'string') return JSON.stringify(value);
  if (Array.isArray(value)) {
    const nonEmpty = value.filter((v) => !isEmptyValue(v));
    if (nonEmpty.length === 0) return '[]';
    const items = nonEmpty.map((v) => hclIndent(depth + 1) + toHcl(v, depth + 1));
    return '[\n' + items.join(',\n') + '\n' + hclIndent(depth) + ']';
  }
  if (typeof value === 'object') {
    // 键字母序排序,保证任意输入下输出顺序一致(diff 对齐与展示都依赖此规范)。
    const entries = Object.entries(value)
      .filter(([_, v]) => !isEmptyValue(v))
      .sort((a, b) => (a[0] < b[0] ? -1 : a[0] > b[0] ? 1 : 0));
    if (entries.length === 0) return '{}';
    const items = entries.map(([k, v]) => hclIndent(depth + 1) + `${hclKey(k)} = ${toHcl(v, depth + 1)}`);
    return '{\n' + items.join('\n') + '\n' + hclIndent(depth) + '}';
  }
  return JSON.stringify(value);
};

// 若 value 是结构化数据(JSON 字符串或实际对象/数组),转 HCL 展示;否则返回 null。
const tryHcl = (value: any): string | null => {
  if (typeof value === 'string') {
    const trimmed = value.trim();
    if (!trimmed.startsWith('{') && !trimmed.startsWith('[')) return null;
    try {
      const parsed = JSON.parse(trimmed);
      if (typeof parsed !== 'object' || parsed === null) return null;
      return toHcl(parsed, 0);
    } catch {
      return null;
    }
  }
  if (value !== null && typeof value === 'object') {
    return toHcl(value, 0);
  }
  return null;
};

// key 列宽度按给定键里最长的那个来(CSS 变量),用于一级参数对齐。
// extra = key 后分隔符长度(: 为 1," =" 为 2),让最长那行也被 padding 到统一列宽。
const keyColVar = (keys: string[], extra = 1): React.CSSProperties => {
  const max = keys.reduce((m, k) => Math.max(m, k.length), 0);
  return { ['--attr-key-col' as any]: `${Math.max(max, 4) + extra}ch` };
};

// ===== UPDATE/REPLACE 结构化 diff:normalize -> 排序 -> 递归 diff -> 行级标记 HCL =====

// normalize:把"长得像 JSON 的字符串"(jsonencode 等)解析回对象/数组;其余原样。
const normalize = (v: any): any => {
  if (typeof v === 'string') {
    const t = v.trim();
    if (t.startsWith('{') || t.startsWith('[')) {
      try {
        const p = JSON.parse(t);
        if (typeof p === 'object' && p !== null) return p;
      } catch { /* 非合法 JSON,当普通字符串 */ }
    }
  }
  return v;
};

// 规范化(键排序)序列化,用于与键顺序无关的深比较。
const canon = (v: any): string => {
  const n = normalize(v);
  if (n === null || n === undefined) return 'null';
  if (Array.isArray(n)) return '[' + n.map(canon).join(',') + ']';
  if (typeof n === 'object') {
    return '{' + Object.keys(n).sort().map(k => JSON.stringify(k) + ':' + canon(n[k])).join(',') + '}';
  }
  return JSON.stringify(n);
};
const deepEqual = (a: any, b: any): boolean => canon(a) === canon(b);

// after_unknown 子图里某个位置为真,表示该字段"known after apply"。
// 布尔 true = 标量未知;非空对象/数组 = 子级里存在未知字段。空对象/数组视为无未知子字段。
const isUnknownFlag = (v: any): boolean => {
  if (v === true) return true;
  if (Array.isArray(v)) return v.some(x => isUnknownFlag(x));
  if (v !== null && typeof v === 'object') return Object.keys(v).some(k => isUnknownFlag(v[k]));
  return false;
};

type DiffKind = 'unchanged' | 'added' | 'removed' | 'modified';
interface DiffNode {
  kind: DiffKind;
  before?: any;
  after?: any;
  entries?: { key: string; node: DiffNode }[]; // 对象容器
  items?: DiffNode[];                          // 数组容器
}
const isPlainObj = (v: any): boolean => v !== null && typeof v === 'object' && !Array.isArray(v);

// 递归 diff:before/after 各自 normalize 后按类型比较。
// 对象:keys 并集字母序排序后逐 key 递归;数组:按索引配对,余量标 added/removed。
// auRaw = 当前层级的 after_unknown 子图,用于判定"只在 before 的 key"是否为 known after apply。
const diffNode = (beforeRaw: any, afterRaw: any, auRaw: any = {}): DiffNode => {
  const before = normalize(beforeRaw);
  const after = normalize(afterRaw);
  const au = auRaw ?? {};
  if (deepEqual(before, after)) {
    // 值相等但 after_unknown 标了未知 -> 当作 modified(渲染 -> known after apply),不能折叠成 unchanged。
    // (Terraform 常用旧值占位,只靠 before/after 相等会漏掉计算字段。)
    return isUnknownFlag(au) ? { kind: 'modified', before, after: undefined } : { kind: 'unchanged', before, after };
  }
  if (isPlainObj(before) && isPlainObj(after)) {
    const keys = Array.from(new Set([...Object.keys(before), ...Object.keys(after)])).sort();
    const entries = keys.map(k => {
      const inB = k in before, inA = k in after;
      // 只在一侧的 key 直接标 added/removed,不走 diffNode(undefined, …)
      // (否则 before=undefined 会让标量分支误判成 modified,渲染出 "Known after apply")。
      // 但"只在 before"的 key 若被 after_unknown 标记,是 known after apply 而非删除。
      const node = !inB ? { kind: 'added' as DiffKind, after: after[k] }
        : !inA ? (isUnknownFlag(au[k])
            ? { kind: 'modified' as DiffKind, before: before[k], after: undefined }
            : { kind: 'removed' as DiffKind, before: before[k] })
          : diffNode(before[k], after[k], au[k]);
      return { key: k, node };
    });
    return { kind: entries.every(e => e.node.kind === 'unchanged') ? 'unchanged' : 'modified', entries };
  }
  if (Array.isArray(before) && Array.isArray(after)) {
    const len = Math.max(before.length, after.length);
    const items: DiffNode[] = [];
    for (let i = 0; i < len; i++) {
      const inB = i < before.length, inA = i < after.length;
      items.push(inB && inA ? diffNode(before[i], after[i], Array.isArray(au) ? au[i] : undefined)
        : inA ? { kind: 'added', after: after[i] }
          : (isUnknownFlag(Array.isArray(au) ? au[i] : undefined)
              ? { kind: 'modified', before: before[i], after: undefined }
              : { kind: 'removed', before: before[i] }));
    }
    return { kind: items.every(n => n.kind === 'unchanged') ? 'unchanged' : 'modified', items };
  }
  // 标量不同 / 类型不同
  return { kind: 'modified', before, after };
};

// diff 行级标记(+/-/~)与颜色。
const diffMarker = (kind: 'add' | 'remove' | 'modify') => {
  const cls = kind === 'add' ? styles.iconAdd : kind === 'remove' ? styles.iconRemove : styles.iconModify;
  const ch = kind === 'add' ? '+' : kind === 'remove' ? '−' : '~';
  return <span className={`${styles.changeIcon} ${cls}`}>{ch}</span>;
};
// 结构化值转 HCL,标量走 formatSimpleValue。
const diffValue = (v: any, cls: string) => {
  const hcl = tryHcl(v);
  return <span className={cls}>{hcl ?? formatSimpleValue(v)}</span>;
};

// modified 标量/类型不同的内联比较:before -> after。
// before 空:直接 known after apply;before 非空:渲染 before 块,把 -> 和(after 值 | known after apply)
// 贴到块的最后一行(diffTrailing vertical-align: bottom),避免多行块时标记跑到第一行。
const renderInlineCompare = (before: any, after: any): React.ReactNode => {
  if (isEmptyValue(before)) {
    return <span className={styles.diffInlineCompare}><span className={styles.knownAfterApply}>known after apply</span></span>;
  }
  const afterEmpty = isEmptyValue(after);
  return (
    <span className={styles.diffInlineCompare}>
      {diffValue(before, styles.diffValueBefore)}
      <span className={styles.diffTrailing}>
        {' -> '}
        {afterEmpty
          ? <span className={styles.knownAfterApply}>known after apply</span>
          : diffValue(after, styles.jsonValueAfter)}
      </span>
    </span>
  );
};

const PlanCompleteView: React.FC<Props> = ({ resources, outputChanges = [], actionInvocations = [], workspaceId, taskId }) => {
  const [expandedResources, setExpandedResources] = useState<Set<number>>(new Set());
  const [showUnchanged, setShowUnchanged] = useState<Set<number>>(new Set());
  const [searchTerm, setSearchTerm] = useState('');
  const [selectedActions, setSelectedActions] = useState<Set<string>>(new Set(['create', 'update', 'delete', 'replace']));
  const [showActionFilter, setShowActionFilter] = useState(false);
  const [copySuccess, setCopySuccess] = useState<string | null>(null);
  const [outputsExpanded, setOutputsExpanded] = useState(false);
  const [actionsExpanded, setActionsExpanded] = useState(true);
  const [expandedActionIndices, setExpandedActionIndices] = useState<Set<number>>(new Set());

  // 计算每个action的数量
  const actionCounts = useMemo(() => ({
    create: resources.filter(r => r.action === 'create').length,
    update: resources.filter(r => r.action === 'update').length,
    delete: resources.filter(r => r.action === 'delete').length,
    replace: resources.filter(r => r.action === 'replace').length,
    read: resources.filter(r => r.action === 'read').length,
  }), [resources]);

  // 创建从触发资源地址到 action invocations 的映射
  const triggerToActionsMap = useMemo(() => {
    const map = new Map<string, ActionInvocation[]>();
    actionInvocations.forEach(action => {
      const triggerResource = action.lifecycle_action_trigger?.triggering_resource_address;
      if (triggerResource) {
        const existing = map.get(triggerResource) || [];
        existing.push(action);
        map.set(triggerResource, existing);
      }
    });
    return map;
  }, [actionInvocations]);

  const allActions = ['create', 'update', 'delete', 'replace', 'read'];

  // 过滤资源
  const filteredResources = useMemo(() => {
    return resources.filter(resource => {
      const matchesSearch = !searchTerm ||
        resource.resource_address.toLowerCase().includes(searchTerm.toLowerCase()) ||
        resource.resource_type.toLowerCase().includes(searchTerm.toLowerCase());
      const matchesAction = selectedActions.size === allActions.length || selectedActions.has(resource.action);
      return matchesSearch && matchesAction;
    });
  }, [resources, searchTerm, selectedActions]);

  // 点击外部关闭下拉菜单
  React.useEffect(() => {
    const handleClickOutside = (event: MouseEvent) => {
      const target = event.target as HTMLElement;
      if (!target.closest(`.${styles.actionFilterContainer}`)) {
        setShowActionFilter(false);
      }
    };
    if (showActionFilter) {
      document.addEventListener('click', handleClickOutside);
      return () => document.removeEventListener('click', handleClickOutside);
    }
  }, [showActionFilter]);

  const toggleResource = (id: number) => {
    setExpandedResources(prev => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  const toggleUnchanged = (id: number) => {
    setShowUnchanged(prev => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  const toggleAction = (action: string) => {
    setSelectedActions(prev => {
      const next = new Set(prev);
      if (next.has(action)) next.delete(action);
      else next.add(action);
      return next;
    });
  };

  // 复制到剪贴板（兼容 HTTP 非安全上下文）
  const copyToClipboard = async (text: string, label: string) => {
    try {
      if (navigator.clipboard && window.isSecureContext) {
        await navigator.clipboard.writeText(text);
      } else {
        const textarea = document.createElement('textarea');
        textarea.value = text;
        textarea.style.position = 'fixed';
        textarea.style.opacity = '0';
        document.body.appendChild(textarea);
        textarea.select();
        document.execCommand('copy');
        document.body.removeChild(textarea);
      }
      setCopySuccess(`Copied ${label}`);
      setTimeout(() => setCopySuccess(null), 2000);
    } catch (err) {
      console.error('Failed to copy:', err);
      setCopySuccess('Failed to copy');
      setTimeout(() => setCopySuccess(null), 2000);
    }
  };

  const renderValue = (value: any, variant: 'create' | 'delete' | 'before' | 'after' | 'neutral' = 'neutral') => {
    if (isEmptyValue(value) && (variant === 'create' || variant === 'after')) {
      return <span className={styles.knownAfterApply}>Known after apply</span>;
    }

    const className = {
      create: styles.valueCreate,
      delete: styles.valueDelete,
      before: styles.valueBefore,
      after: styles.valueAfter,
      neutral: styles.valueNeutral,
    }[variant];

    // 字符串值,但内容是合法 JSON 对象/数组 -> 转 HCL 展示。
    // 颜色按 variant:delete 用红,create/其余用绿。
    const hcl = tryHcl(value);
    if (hcl) {
      const hclClass = variant === 'delete' ? styles.jsonValueDelete : styles.jsonValue;
      return <span className={hclClass}>{hcl}</span>;
    }

    return <span className={className}>{formatSimpleValue(value)}</span>;
  };

  // 渲染 CREATE 资源的属性
  const renderCreateBody = (resource: ResourceChange) => {
    const after = resource.changes_after || {};
    const afterUnknown = resource.after_unknown || {};

    const knownEntries = Object.entries(after).filter(([k, v]) => !isEmptyValue(v) && afterUnknown[k] !== true);
    const knownKeySet = new Set(knownEntries.map(([k]) => k));
    // 已经以真实值显示过的字段不再重复显示为 "Known after apply"
    const unknownKeys = Object.keys(afterUnknown).filter(k => !!afterUnknown[k] && !knownKeySet.has(k));

    if (knownEntries.length === 0 && unknownKeys.length === 0) {
      return null;
    }

    return (
      <div style={keyColVar([...knownEntries.map(([k]) => k), ...unknownKeys], 1)}>
        <div className={styles.sectionLabel}>Resource will be created:</div>

        {knownEntries.length > 0 && (
          <div className={styles.simpleAttrsGrid}>
            {knownEntries.map(([key, value]) => (
              <div key={key} className={styles.simpleAttrRow}>
                <span className={styles.attrIcon}>+</span>
                <span className={styles.attrKey}>{key}:</span>
                {renderValue(value, 'create')}
              </div>
            ))}
          </div>
        )}

        {unknownKeys.length > 0 && (
          <div className={styles.simpleAttrsGrid}>
            {unknownKeys.map(key => (
              <div key={key} className={styles.simpleAttrRow}>
                <span className={styles.attrIcon}>+</span>
                <span className={styles.attrKey}>{key}:</span>
                <span className={styles.knownAfterApply}>Known after apply</span>
              </div>
            ))}
          </div>
        )}
      </div>
    );
  };

  // 渲染 DELETE 资源的属性
  const renderDeleteBody = (resource: ResourceChange) => {
    const before = resource.changes_before || {};
    const entries = Object.entries(before).filter(([_, v]) => !isEmptyValue(v));
    
    if (entries.length === 0) {
      return <div className={styles.emptyMessage}>No attributes to display</div>;
    }

    return (
      <div style={keyColVar(entries.map(([k]) => k), 1)}>
        <div className={`${styles.sectionLabel} ${styles.sectionLabelDelete}`}>Resource will be destroyed:</div>

        <div className={styles.simpleAttrsGrid}>
          {entries.map(([key, value]) => (
            <div key={key} className={styles.simpleAttrRow}>
              <span className={`${styles.attrIcon} ${styles.attrIconDelete}`}>−</span>
              <span className={styles.attrKey}>{key}:</span>
              {renderValue(value, 'delete')}
            </div>
          ))}
        </div>
      </div>
    );
  };

  // 渲染 UPDATE/REPLACE 资源的属性 —— 结构化深 diff,行级标记 HCL。
  // before/after 先各自 normalize(jsonencode 等字符串 JSON 解析回对象),
  // 再递归 diff:对象键字母序排序后逐键比较,数组按索引配对。
  // 变更直接展示;未变更默认折叠,点击展开时扁平铺开(不用滚动窗口)。
  const renderUpdateBody = (resource: ResourceChange) => {
    const afterUnknown = resource.after_unknown || {};
    const root = diffNode(resource.changes_before || {}, resource.changes_after || {}, resource.after_unknown || {});
    const unchanged: Array<{ path: string; value: any }> = [];

    // 渲染 diff 节点。unchanged 收集到 unchanged 列表(默认折叠,展开时扁平铺开)。
    const renderEntry = (node: DiffNode, key: string, depth: number, parentPath: string): React.ReactNode => {
      const path = parentPath ? `${parentPath}.${key}` : key;
      const pad = { paddingLeft: `${12 + depth * 20}px` };

      if (node.kind === 'unchanged') {
        unchanged.push({ path, value: node.after ?? node.before });
        return null;
      }
      if (node.kind === 'added') {
        return (
          <div key={path} className={styles.diffRow} style={pad}>
            {diffMarker('add')}
            <span className={styles.diffKey}>{key} =</span>
            {diffValue(node.after, styles.jsonValue)}
          </div>
        );
      }
      if (node.kind === 'removed') {
        return (
          <div key={path} className={styles.diffRow} style={pad}>
            {diffMarker('remove')}
            <span className={styles.diffKey}>{key} =</span>
            {diffValue(node.before, styles.jsonValueDelete)}
          </div>
        );
      }
      // modified
      if (node.entries) {
        const children = node.entries
          .map(ce => renderEntry(ce.node, ce.key, depth + 1, path))
          .filter(Boolean);
        if (children.length === 0) return null;
        return (
          <div key={path}>
            <div className={styles.diffRow} style={pad}>
              {diffMarker('modify')}
              <span className={styles.diffKey}>{key} =</span>
              <span className={styles.diffBrace}>{'{'}</span>
            </div>
            {children}
            <div className={styles.diffRow} style={pad}><span className={styles.diffBrace}>{'}'}</span></div>
          </div>
        );
      }
      if (node.items) {
        const children = node.items
          .map((it, i) => renderArrayItem(it, depth + 1, `${path}[${i}]`))
          .filter(Boolean);
        if (children.length === 0) return null;
        return (
          <div key={path}>
            <div className={styles.diffRow} style={pad}>
              {diffMarker('modify')}
              <span className={styles.diffKey}>{key} =</span>
              <span className={styles.diffBrace}>{'['}</span>
            </div>
            {children}
            <div className={styles.diffRow} style={pad}><span className={styles.diffBrace}>{']'}</span></div>
          </div>
        );
      }
      // 标量 / 类型不同的修改:内联 before -> after
      return (
        <div key={path} className={styles.diffRow} style={pad}>
          {diffMarker('modify')}
          <span className={styles.diffKey}>{key} =</span>
          {renderInlineCompare(node.before, node.after)}
        </div>
      );
    };

    // 数组元素(无 key):整体值块或 ~ before -> after。unchanged 收集到列表(折叠)。
    const renderArrayItem = (node: DiffNode, depth: number, path: string): React.ReactNode => {
      const pad = { paddingLeft: `${12 + depth * 20}px` };
      if (node.kind === 'unchanged') {
        unchanged.push({ path, value: node.after ?? node.before });
        return null;
      }
      if (node.kind === 'added') {
        return (
          <div key={path} className={styles.diffRow} style={pad}>
            {diffMarker('add')}
            {diffValue(node.after, styles.jsonValue)}
          </div>
        );
      }
      if (node.kind === 'removed') {
        return (
          <div key={path} className={styles.diffRow} style={pad}>
            {diffMarker('remove')}
            {diffValue(node.before, styles.jsonValueDelete)}
          </div>
        );
      }
      if (node.entries) {
        const children = node.entries
          .map(ce => renderEntry(ce.node, ce.key, depth + 1, path))
          .filter(Boolean);
        return (
          <div key={path}>
            <div className={styles.diffRow} style={pad}>
              {diffMarker('modify')}
              <span className={styles.diffBrace}>{'{'}</span>
            </div>
            {children}
            <div className={styles.diffRow} style={pad}><span className={styles.diffBrace}>{'}'}</span></div>
          </div>
        );
      }
      if (node.items) {
        const children = node.items
          .map((it, i) => renderArrayItem(it, depth + 1, `${path}[${i}]`))
          .filter(Boolean);
        return (
          <div key={path}>
            <div className={styles.diffRow} style={pad}>
              {diffMarker('modify')}
              <span className={styles.diffBrace}>{'['}</span>
            </div>
            {children}
            <div className={styles.diffRow} style={pad}><span className={styles.diffBrace}>{']'}</span></div>
          </div>
        );
      }
      // 标量元素修改
      return (
        <div key={path} className={styles.diffRow} style={pad}>
          {diffMarker('modify')}
          {renderInlineCompare(node.before, node.after)}
        </div>
      );
    };

    const rows = (root.entries || [])
      .map(e => renderEntry(e.node, e.key, 0, ''))
      .filter(Boolean);

    // after_unknown 中 before/after 都没有的字段:known after apply。
    const allKeys = new Set((root.entries || []).map(e => e.key));
    const extraUnknownKeys = Object.keys(afterUnknown).filter(k => !!afterUnknown[k] && !allKeys.has(k));
    const unknownRows = extraUnknownKeys.map(key => (
      <div key={`u-${key}`} className={styles.diffRow} style={{ paddingLeft: '12px' }}>
        {diffMarker('modify')}
        <span className={styles.diffKey}>{key} =</span>
        <span className={styles.knownAfterApply}>Known after apply</span>
      </div>
    ));

    // 整体无变更(根节点 unchanged)且没有纯计算字段 -> No changes detected。
    if (root.kind === 'unchanged' && unknownRows.length === 0) {
      return <div className={styles.emptyMessage}>No changes detected</div>;
    }

    return (
      <div className={styles.diffTree}>
        <div className={styles.sectionLabel}>Changed fields:</div>
        {rows}
        {unknownRows}

        {/* 未变更字段:默认折叠,展开时扁平铺开(无滚动窗口) */}
        {unchanged.length > 0 && (
          <>
            <div
              className={styles.unchangedToggle}
              onClick={() => toggleUnchanged(resource.id)}
            >
              <span className={styles.toggleIcon}>
                {showUnchanged.has(resource.id) ? '∨' : '›'}
              </span>
              {showUnchanged.has(resource.id)
                ? `Hide ${unchanged.length} unchanged elements`
                : `Show ${unchanged.length} unchanged elements`}
            </div>

            {showUnchanged.has(resource.id) && (
              <div className={styles.unchangedList}>
                {unchanged.map(u => (
                  <div key={u.path} className={styles.unchangedRow}>
                    <span className={styles.unchangedKey}>{u.path}:</span>
                    <span className={styles.unchangedValue}>
                      {tryHcl(u.value) ?? formatSimpleValue(u.value)}
                    </span>
                  </div>
                ))}
              </div>
            )}
          </>
        )}
      </div>
    );
  };

  const getActionConfig = (action: string) => {
    switch (action) {
      case 'create': return { icon: '+', label: 'CREATE', className: styles.actionCreate };
      case 'update': return { icon: '~', label: 'UPDATE', className: styles.actionUpdate };
      case 'delete': return { icon: '−', label: 'DELETE', className: styles.actionDelete };
      case 'replace': return { icon: '±', label: 'REPLACE', className: styles.actionReplace };
      case 'read': return { icon: '≡', label: 'READ', className: styles.actionRead };
      default: return { icon: '?', label: action.toUpperCase(), className: '' };
    }
  };

  return (
    <div className={styles.planComplete}>
      {copySuccess && <div className={styles.copyToast}>{copySuccess}</div>}
      
      {/* 过滤栏 */}
      <div className={styles.filterBar}>
        <div className={styles.filterRow}>
          <div className={styles.searchBox}>
            <svg className={styles.searchIcon} width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
              <circle cx="11" cy="11" r="8"></circle>
              <path d="M21 21l-4.35-4.35"></path>
            </svg>
            <input
              type="text"
              placeholder="Filter resources by address..."
              value={searchTerm}
              onChange={(e) => setSearchTerm(e.target.value)}
              className={styles.searchInput}
            />
            {searchTerm && (
              <button onClick={() => setSearchTerm('')} className={styles.clearButton}>✕</button>
            )}
          </div>

          <div className={styles.actionFilterContainer}>
            <button
              className={styles.actionFilterButton}
              onClick={() => setShowActionFilter(!showActionFilter)}
            >
              <span className={styles.filterIcon}>☰</span>
              Filter
              {selectedActions.size > 0 && (
                <span className={styles.filterCount}>{selectedActions.size}</span>
              )}
              <span className={styles.chevron}>{showActionFilter ? '▲' : '∨'}</span>
            </button>

            {showActionFilter && (
              <div className={styles.actionFilterDropdown}>
                {['create', 'update', 'delete', 'replace', 'read'].map(action => (
                  <label key={action} className={styles.actionFilterItem}>
                    <input 
                      type="checkbox" 
                      checked={selectedActions.has(action)} 
                      onChange={() => toggleAction(action)} 
                    />
                    <span className={styles.actionFilterLabel}>
                      <span className={styles[`icon${action.charAt(0).toUpperCase() + action.slice(1)}`]}>
                        {getActionConfig(action).icon}
                      </span>
                      {action.charAt(0).toUpperCase() + action.slice(1)} ({actionCounts[action as keyof typeof actionCounts]})
                    </span>
                  </label>
                ))}
              </div>
            )}
          </div>
          {workspaceId && taskId && (
            <button
              className={styles.planDownloadBtn}
              onClick={async () => {
                try {
                  const data = await api.get(`/workspaces/${workspaceId}/tasks/${taskId}/resource-changes`);
                  const blob = new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' });
                  const url = URL.createObjectURL(blob);
                  const a = document.createElement('a');
                  a.href = url;
                  a.download = `plan-changes-task-${taskId}.json`;
                  a.click();
                  URL.revokeObjectURL(url);
                } catch (err) {
                  console.error('Failed to download plan changes:', err);
                }
              }}
            >
              plan changes download
            </button>
          )}
        </div>

        {(searchTerm || selectedActions.size < allActions.length) && (
          <div className={styles.filterResult}>
            Showing {filteredResources.length} of {resources.length} resources
            <button
              onClick={() => { setSearchTerm(''); setSelectedActions(new Set(allActions)); }}
              className={styles.clearFiltersButton}
            >
              Clear filters
            </button>
          </div>
        )}
      </div>

      {/* 资源列表 */}
      <div className={styles.resourceList}>
        {filteredResources.map((resource) => {
          const isExpanded = expandedResources.has(resource.id);
          const config = getActionConfig(resource.action);
          // 检查这个资源是否会触发 actions
          const triggeredActions = triggerToActionsMap.get(resource.resource_address) || [];

          return (
            <div key={resource.id} className={`${styles.resourceCard} ${config.className}`}>
              <div className={styles.resourceHeader} onClick={() => toggleResource(resource.id)}>
                <div className={styles.resourceHeaderLeft}>
                  <span className={styles.expandIcon}>{isExpanded ? '∨' : '›'}</span>
                  <span className={`${styles.actionIcon} ${config.className}`}>{config.icon}</span>
                  <span className={styles.resourceAddress}>{resource.resource_address}</span>
                  <button
                    className={styles.copyButton}
                    onClick={(e) => { e.stopPropagation(); copyToClipboard(resource.resource_address, 'address'); }}
                    title="Copy resource address"
                  >
                    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                      <rect x="9" y="9" width="13" height="13" rx="2" ry="2"></rect>
                      <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"></path>
                    </svg>
                  </button>
                  {/* 显示触发的 actions */}
                  {triggeredActions.length > 0 && (
                    <span className={styles.triggersActionIndicator} title={`Triggers ${triggeredActions.length} action(s)`}>
                      <svg width="16" height="16" viewBox="0 0 24 16" fill="none" stroke="currentColor" strokeWidth="2">
                        <path d="M0 8h16M12 4l4 4-4 4" />
                      </svg>
                      <span className={styles.triggersActionBadge}>
                        {triggeredActions.map(a => a.type).join(', ')}
                      </span>
                    </span>
                  )}
                </div>
                <div className={styles.resourceHeaderRight}>
                  <span className={styles.resourceTypeTag}>{resource.resource_type}</span>
                </div>
              </div>

              {isExpanded && (
                <div className={styles.resourceBody}>
                  {(resource.action === 'create' || resource.action === 'read') && renderCreateBody(resource)}
                  {resource.action === 'delete' && renderDeleteBody(resource)}
                  {(resource.action === 'update' || resource.action === 'replace') && renderUpdateBody(resource)}
                  
                  {/* 显示触发的 actions 详情 */}
                  {triggeredActions.length > 0 && (
                    <div className={styles.triggeredActionsSection}>
                      <div className={styles.triggeredActionsLabel}>
                        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" style={{ marginRight: '6px', verticalAlign: 'middle' }}>
                          <polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2"></polygon>
                        </svg>
                        Triggers Actions:
                      </div>
                      {triggeredActions.map((action, idx) => {
                        const triggerEvent = action.lifecycle_action_trigger?.action_trigger_event || 'Unknown';
                        return (
                          <div key={idx} className={styles.triggeredActionItem}>
                            <span className={styles.triggeredActionArrow}>→</span>
                            <span className={styles.triggeredActionAddress} title={action.address}>{action.address}</span>
                            <button
                              className={styles.copyButton}
                              onClick={(e) => { e.stopPropagation(); copyToClipboard(action.address, 'action address'); }}
                              title="Copy action address"
                            >
                              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                                <rect x="9" y="9" width="13" height="13" rx="2" ry="2"></rect>
                                <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"></path>
                              </svg>
                            </button>
                            <span className={styles.triggeredActionType}>{action.type}</span>
                            <span
                              className={styles.triggeredActionEvent}
                              style={triggerEventStyle(triggerEvent)}
                            >
                              {formatTriggerEvent(triggerEvent)}
                            </span>
                          </div>
                        );
                      })}
                    </div>
                  )}
                </div>
              )}
            </div>
          );
        })}
      </div>

      {/* Action Invocations (Terraform 1.14+ 新特性) */}
      {actionInvocations.length > 0 && (
        <div className={styles.actionInvocationsSection}>
          <div className={styles.actionInvocationsHeader} onClick={() => setActionsExpanded(!actionsExpanded)}>
            <span className={styles.expandIcon}>{actionsExpanded ? '∨' : '›'}</span>
            <span className={styles.actionInvocationsTitle}>
              <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" style={{ marginRight: '6px', verticalAlign: 'middle' }}>
                <polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2"></polygon>
              </svg>
              Action Invocations
            </span>
            <span className={styles.actionInvocationsCount}>{actionInvocations.length}</span>
            <span className={styles.actionInvocationsBadge}>Terraform 1.14+</span>
          </div>
          {actionsExpanded && (
            <div className={styles.actionInvocationsList}>
              {actionInvocations.map((action, index) => {
                const isExpanded = expandedActionIndices.has(index);
                const triggerEvent = action.lifecycle_action_trigger?.action_trigger_event || 'Unknown';
                const triggerResource = action.lifecycle_action_trigger?.triggering_resource_address || '';

                return (
                  <div key={index} className={styles.actionInvocationItem}>
                    <div 
                      className={styles.actionInvocationHeader}
                      onClick={() => {
                        setExpandedActionIndices(prev => {
                          const next = new Set(prev);
                          if (next.has(index)) next.delete(index);
                          else next.add(index);
                          return next;
                        });
                      }}
                    >
                      <div className={styles.actionInvocationLeft}>
                        <span className={styles.expandIcon}>{isExpanded ? '∨' : '›'}</span>
                        <span className={styles.actionInvocationAddress}>{action.address}</span>
                        <button
                          className={styles.copyButton}
                          onClick={(e) => { e.stopPropagation(); copyToClipboard(action.address, 'action address'); }}
                          title="Copy action address"
                        >
                          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                            <rect x="9" y="9" width="13" height="13" rx="2" ry="2"></rect>
                            <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"></path>
                          </svg>
                        </button>
                      </div>
                      <div className={styles.actionInvocationRight}>
                        <span className={styles.actionInvocationTypeTag}>{action.type}</span>
                        <span
                          className={styles.actionInvocationTriggerBadge}
                          style={triggerEventStyle(triggerEvent)}
                        >
                          {formatTriggerEvent(triggerEvent)}
                        </span>
                      </div>
                    </div>
                    
                    {isExpanded && (
                      <div className={styles.actionInvocationBody}>
                        {/* 触发关系 */}
                        {triggerResource && (
                          <div className={styles.actionTriggerRelation}>
                            <div className={styles.triggerRelationLabel}>Triggered by:</div>
                            <div className={styles.triggerRelationFlow}>
                              <span className={styles.triggerResourceAddress}>{triggerResource}</span>
                            </div>
                          </div>
                        )}
                        
                        {/* Action 配置值:扁平行 + key 列对齐(与资源详情一致),值走 HCL */
                        action.config_values && Object.keys(action.config_values).length > 0 && (
                          <div className={styles.actionConfigSection}>
                            <div className={styles.actionConfigLabel}>Configuration:</div>
                            <div className={styles.simpleAttrsGrid} style={keyColVar(Object.keys(action.config_values), 1)}>
                              {Object.entries(action.config_values).map(([key, value]) => {
                                // 跳过空值和空数组
                                if (value === null || (Array.isArray(value) && value.length === 0)) {
                                  // 检查是否在 config_unknown 中
                                  if (action.config_unknown?.[key]) {
                                    return (
                                      <div key={key} className={styles.simpleAttrRow}>
                                        <span className={styles.attrKey}>{key}:</span>
                                        <span className={styles.knownAfterApply}>(known after apply)</span>
                                      </div>
                                    );
                                  }
                                  return null;
                                }
                                return (
                                  <div key={key} className={styles.simpleAttrRow}>
                                    <span className={styles.attrKey}>{key}:</span>
                                    <span className={styles.actionConfigValue}>{tryHcl(value) ?? formatSimpleValue(value)}</span>
                                  </div>
                                );
                              })}
                            </div>
                          </div>
                        )}
                        
                        {/* Provider 信息 */}
                        {action.provider_name && (
                          <div className={styles.actionProviderInfo}>
                            <span className={styles.actionProviderLabel}>Provider:</span>
                            <span className={styles.actionProviderValue}>{action.provider_name}</span>
                          </div>
                        )}
                      </div>
                    )}
                  </div>
                );
              })}
            </div>
          )}
        </div>
      )}

      {/* Output Changes */}
      {outputChanges.length > 0 && (() => {
        // 过滤掉没有实际变更的 outputs（before 和 after 相同）
        const actualChanges = outputChanges.filter(output => {
          // no-op 表示没有变更
          if (output.action === 'no-op') return false;
          // 如果 before 和 after 相同，也不显示
          if (JSON.stringify(output.before) === JSON.stringify(output.after)) return false;
          return true;
        });
        
        if (actualChanges.length === 0) return null;
        
        return (
          <div className={styles.outputChangesSection}>
            <div className={styles.outputChangesHeader} onClick={() => setOutputsExpanded(!outputsExpanded)}>
              <span className={styles.expandIcon}>{outputsExpanded ? '∨' : '›'}</span>
              <span className={styles.outputChangesTitle}>Output Changes</span>
              <span className={styles.outputChangesCount}>{actualChanges.length}</span>
            </div>
            {outputsExpanded && (
              <div className={styles.outputChangesList}>
                {actualChanges.map((output, index) => (
                  <div key={index} className={styles.outputChangeItem}>
                    <div className={styles.outputChangeHeader}>
                      <span className={`${styles.actionIcon} ${styles[`action${output.action.charAt(0).toUpperCase() + output.action.slice(1)}`]}`}>
                        {output.action === 'create' ? '+' : output.action === 'delete' ? '−' : '~'}
                      </span>
                      <span className={styles.outputName}>{output.name}</span>
                      {output.sensitive && <span className={styles.sensitiveTag}>sensitive</span>}
                    </div>
                    <div className={styles.outputChangeValue}>
                      {output.sensitive ? (
                        // Sensitive outputs: always show masked value
                        <span className={styles.sensitiveValue}>(sensitive value)</span>
                      ) : output.action === 'create' ? (
                        output.after_unknown ? (
                          <span className={styles.knownAfterApply}>(known after apply)</span>
                        ) : (
                          renderValue(output.after, 'create')
                        )
                      ) : output.action === 'delete' ? (
                        renderValue(output.before, 'delete')
                      ) : (
                        // Update action
                        <span className={styles.valueComparison}>
                          {renderValue(output.before, 'before')}
                          <span className={styles.arrow}>→</span>
                          {output.after_unknown ? (
                            <span className={styles.knownAfterApply}>(known after apply)</span>
                          ) : (
                            renderValue(output.after, 'after')
                          )}
                        </span>
                      )}
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>
        );
      })()}

      {/* 空状态 */}
      {resources.length === 0 && outputChanges.length === 0 && (
        <div className={styles.noResources}>
          <div className={styles.noResourcesIcon}>
            <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
              <path d="M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2" />
            </svg>
          </div>
          <h3>No Resource Changes Data</h3>
          <p>Unable to load parsed resource changes for this task.</p>
          <p className={styles.noResourcesHint}>
            Possible reasons:<br />
            - Task was created before the feature was implemented<br />
            - Async parsing hasn't completed yet (wait a few seconds)<br />
            - Plan detected no changes
          </p>
        </div>
      )}
    </div>
  );
};

export default PlanCompleteView;
