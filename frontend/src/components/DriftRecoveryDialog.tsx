import React from 'react';
import type { DriftInfo } from '../services/resourceEditing';
import { formatTimeAgo } from '../services/resourceEditing';
import styles from './DriftRecoveryDialog.module.css';

interface DriftRecoveryDialogProps {
  drift: DriftInfo;
  hasVersionConflict: boolean;
  resourceId?: string;
  resourceName?: string;
  onRecover: () => void;
  onDiscard: () => void;
  onCancel: () => void;
}

const DriftRecoveryDialog: React.FC<DriftRecoveryDialogProps> = ({
  drift,
  hasVersionConflict,
  resourceId,
  resourceName,
  onRecover,
  onDiscard,
  onCancel,
}) => {
  return (
    <div className={styles.overlay}>
      <div className={styles.dialog}>
        <div className={styles.header}>
          <h3 className={styles.title}>发现未提交的草稿</h3>
        </div>

        <div className={styles.content}>
          {hasVersionConflict && (
            <div className={styles.warningBox}>
              <span className={styles.warningIcon}></span>
              <div className={styles.warningText}>
                <strong>版本冲突警告</strong>
                <p>
                  资源已被其他用户修改，草稿基于旧版本（v{drift.base_version}）。
                  您可以查看草稿内容，但无法直接提交。
                </p>
              </div>
            </div>
          )}

          <div className={styles.infoBox}>
            {resourceId && (
              <div className={styles.infoRow}>
                <span className={styles.infoLabel}>资源:</span>
                <span className={styles.infoValue}>
                  {resourceName || resourceId}
                </span>
              </div>
            )}
            <div className={styles.infoRow}>
              <span className={styles.infoLabel}>草稿ID:</span>
              <span className={styles.infoValue}>#{drift.id}</span>
            </div>
            <div className={styles.infoRow}>
              <span className={styles.infoLabel}>草稿保存时间:</span>
              <span className={styles.infoValue}>
                {formatTimeAgo(drift.updated_at)}
              </span>
            </div>
            <div className={styles.infoRow}>
              <span className={styles.infoLabel}>基于版本:</span>
              <span className={styles.infoValue}>v{drift.base_version}</span>
            </div>
            {drift.drift_content.changeSummary && (
              <div className={styles.infoRow}>
                <span className={styles.infoLabel}>变更摘要:</span>
                <span className={styles.infoValue}>
                  {drift.drift_content.changeSummary}
                </span>
              </div>
            )}
          </div>

          <p className={styles.description}>
            {hasVersionConflict
              ? '您可以查看草稿内容作为参考，或删除草稿重新开始编辑。'
              : '您可以恢复草稿继续编辑，或删除草稿重新开始。'}
          </p>
        </div>

        <div className={styles.actions}>
          <button
            className={styles.btnSecondary}
            onClick={onCancel}
            type="button"
          >
            取消
          </button>
          <button
            className={styles.btnDanger}
            onClick={onDiscard}
            type="button"
          >
            删除草稿
          </button>
          <button
            className={styles.btnPrimary}
            onClick={onRecover}
            type="button"
          >
            {hasVersionConflict ? '查看草稿内容' : '恢复草稿'}
          </button>
        </div>
      </div>
    </div>
  );
};

export default DriftRecoveryDialog;
