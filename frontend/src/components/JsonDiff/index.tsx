import React, { useMemo } from 'react';
import styles from './JsonDiff.module.css';

interface JsonDiffProps {
  oldJson: any;
  newJson: any;
  oldLabel?: string;
  newLabel?: string;
}

interface DiffLine {
  type: 'unchanged' | 'added' | 'removed' | 'modified';
  path: string;
  oldValue?: any;
  newValue?: any;
  indent: number;
}

// 深度比较两个对象，生成差异
function generateDiff(
  oldObj: any,
  newObj: any,
  path: string = '',
  indent: number = 0
): DiffLine[] {
  const lines: DiffLine[] = [];

  // 处理 null/undefined
  if (oldObj === null || oldObj === undefined) {
    if (newObj === null || newObj === undefined) {
      return [];
    }
    // 新增
    lines.push({
      type: 'added',
      path,
      newValue: newObj,
      indent
    });
    return lines;
  }

  if (newObj === null || newObj === undefined) {
    // 删除
    lines.push({
      type: 'removed',
      path,
      oldValue: oldObj,
      indent
    });
    return lines;
  }

  // 类型不同
  if (typeof oldObj !== typeof newObj) {
    lines.push({
      type: 'modified',
      path,
      oldValue: oldObj,
      newValue: newObj,
      indent
    });
    return lines;
  }

  // 基本类型
  if (typeof oldObj !== 'object') {
    if (oldObj !== newObj) {
      lines.push({
        type: 'modified',
        path,
        oldValue: oldObj,
        newValue: newObj,
        indent
      });
    }
    return lines;
  }

  // 数组
  if (Array.isArray(oldObj) && Array.isArray(newObj)) {
    const maxLen = Math.max(oldObj.length, newObj.length);
    for (let i = 0; i < maxLen; i++) {
      const itemPath = path ? `${path}[${i}]` : `[${i}]`;
      if (i >= oldObj.length) {
        lines.push({
          type: 'added',
          path: itemPath,
          newValue: newObj[i],
          indent
        });
      } else if (i >= newObj.length) {
        lines.push({
          type: 'removed',
          path: itemPath,
          oldValue: oldObj[i],
          indent
        });
      } else {
        lines.push(...generateDiff(oldObj[i], newObj[i], itemPath, indent));
      }
    }
    return lines;
  }

  // 对象
  const allKeys = new Set([...Object.keys(oldObj), ...Object.keys(newObj)]);
  for (const key of allKeys) {
    const keyPath = path ? `${path}.${key}` : key;
    if (!(key in oldObj)) {
      lines.push({
        type: 'added',
        path: keyPath,
        newValue: newObj[key],
        indent
      });
    } else if (!(key in newObj)) {
      lines.push({
        type: 'removed',
        path: keyPath,
        oldValue: oldObj[key],
        indent
      });
    } else {
      lines.push(...generateDiff(oldObj[key], newObj[key], keyPath, indent));
    }
  }

  return lines;
}

// 格式化值显示
function formatValue(value: any): string {
  if (value === null) return 'null';
  if (value === undefined) return 'undefined';
  if (typeof value === 'string') return `"${value}"`;
  if (typeof value === 'object') {
    try {
      return JSON.stringify(value, null, 2);
    } catch {
      return String(value);
    }
  }
  return String(value);
}

// 统计变更
function countChanges(diffs: DiffLine[]): { added: number; removed: number; modified: number } {
  return diffs.reduce(
    (acc, diff) => {
      if (diff.type === 'added') acc.added++;
      else if (diff.type === 'removed') acc.removed++;
      else if (diff.type === 'modified') acc.modified++;
      return acc;
    },
    { added: 0, removed: 0, modified: 0 }
  );
}

export const JsonDiff: React.FC<JsonDiffProps> = ({
  oldJson,
  newJson,
  oldLabel = '旧版本',
  newLabel = '新版本'
}) => {
  const diffs = useMemo(() => generateDiff(oldJson, newJson), [oldJson, newJson]);
  const stats = useMemo(() => countChanges(diffs), [diffs]);

  if (diffs.length === 0) {
    return (
      <div className={styles.noDiff}>
        <span className={styles.noDiffIcon}>✓</span>
        <span>两个版本完全相同，没有差异</span>
      </div>
    );
  }

  return (
    <div className={styles.container}>
      {/* 统计信息 */}
      <div className={styles.stats}>
        <span className={styles.statItem}>
          共 <strong>{diffs.length}</strong> 处变更
        </span>
        {stats.added > 0 && (
          <span className={`${styles.statItem} ${styles.added}`}>
            +{stats.added} 新增
          </span>
        )}
        {stats.removed > 0 && (
          <span className={`${styles.statItem} ${styles.removed}`}>
            -{stats.removed} 删除
          </span>
        )}
        {stats.modified > 0 && (
          <span className={`${styles.statItem} ${styles.modified}`}>
            ~{stats.modified} 修改
          </span>
        )}
      </div>

      {/* 差异列表 */}
      <div className={styles.diffList}>
        {diffs.map((diff, index) => (
          <div key={index} className={`${styles.diffItem} ${styles[diff.type]}`}>
            <div className={styles.diffPath}>
              <span className={styles.diffIcon}>
                {diff.type === 'added' && '+'}
                {diff.type === 'removed' && '-'}
                {diff.type === 'modified' && '~'}
              </span>
              <code className={styles.pathCode}>{diff.path}</code>
            </div>
            <div className={styles.diffContent}>
              {diff.type === 'added' && (
                <div className={styles.valueBox}>
                  <span className={styles.valueLabel}>{newLabel}:</span>
                  <pre className={styles.valueCode}>{formatValue(diff.newValue)}</pre>
                </div>
              )}
              {diff.type === 'removed' && (
                <div className={styles.valueBox}>
                  <span className={styles.valueLabel}>{oldLabel}:</span>
                  <pre className={styles.valueCode}>{formatValue(diff.oldValue)}</pre>
                </div>
              )}
              {diff.type === 'modified' && (
                <>
                  <div className={`${styles.valueBox} ${styles.oldValue}`}>
                    <span className={styles.valueLabel}>{oldLabel}:</span>
                    <pre className={styles.valueCode}>{formatValue(diff.oldValue)}</pre>
                  </div>
                  <div className={styles.arrow}>→</div>
                  <div className={`${styles.valueBox} ${styles.newValue}`}>
                    <span className={styles.valueLabel}>{newLabel}:</span>
                    <pre className={styles.valueCode}>{formatValue(diff.newValue)}</pre>
                  </div>
                </>
              )}
            </div>
          </div>
        ))}
      </div>
    </div>
  );
};

export default JsonDiff;
