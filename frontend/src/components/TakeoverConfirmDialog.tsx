import React, { useState, useEffect } from 'react';
import type { EditorInfo } from '../services/resourceEditing';
import { formatTimeAgo } from '../services/resourceEditing';
import styles from './TakeoverConfirmDialog.module.css';

interface TakeoverConfirmDialogProps {
  otherSession: EditorInfo;
  onConfirm: (forceTakeover: boolean) => void;
  onCancel: () => void;
}

const TakeoverConfirmDialog: React.FC<TakeoverConfirmDialogProps> = ({
  otherSession,
  onConfirm,
  onCancel,
}) => {
  const [forceTakeover, setForceTakeover] = useState(false);
  
  // 同一用户时，直接强制接管，不需要发送请求等待确认
  const isSameUser = otherSession.is_same_user;
  
  return (
    <div className={styles.overlay}>
      <div className={styles.dialog}>
        <div className={styles.header}>
          <h3 className={styles.title}>
            {isSameUser ? '编辑会话冲突' : '接管编辑确认'}
          </h3>
        </div>

        <div className={styles.content}>
          <div className={styles.infoBox}>
            <p className={styles.mainText}>
              {isSameUser 
                ? '您在另一个窗口正在编辑此资源' 
                : `${otherSession.user_name} 正在编辑此资源`}
            </p>
            <div className={styles.detailRow}>
              <span className={styles.label}>最后活动时间:</span>
              <span className={styles.value}>
                {formatTimeAgo(otherSession.last_heartbeat)}
              </span>
            </div>
            <div className={styles.detailRow}>
              <span className={styles.label}>会话ID:</span>
              <span className={styles.value}>
                {otherSession.session_id.substring(0, 8)}...
              </span>
            </div>
          </div>

          {isSameUser ? (
            // 同一用户：简化提示，直接确认即可
            <div className={styles.warningBox} style={{ background: '#dbeafe', borderColor: '#3b82f6' }}>
              <span className={styles.warningIcon}>ℹ️</span>
              <p className={styles.warningText} style={{ color: '#1e40af' }}>
                确认后将关闭另一个窗口的编辑会话，在此窗口继续编辑。
              </p>
            </div>
          ) : (
            // 不同用户：显示完整的接管选项
            <>
              <div className={styles.warningBox}>
                <span className={styles.warningIcon}></span>
                <p className={styles.warningText}>
                  接管后，对方窗口将收到通知并需要确认。
                  <br />
                  对方有30秒时间决定是否同意接管。
                </p>
              </div>

              <div style={{ marginTop: '16px', padding: '12px', background: '#fef3c7', borderRadius: '8px' }}>
                <label style={{ display: 'flex', alignItems: 'center', cursor: 'pointer' }}>
                  <input
                    type="checkbox"
                    checked={forceTakeover}
                    onChange={(e) => setForceTakeover(e.target.checked)}
                    style={{ marginRight: '8px', width: '16px', height: '16px', cursor: 'pointer' }}
                  />
                  <span style={{ fontSize: '14px', color: '#92400e' }}>
                    <strong>强制接管</strong> - 立即踢掉对方，无需等待确认
                  </span>
                </label>
              </div>
            </>
          )}

          <p className={styles.question}>
            {isSameUser 
              ? '确定要在此窗口继续编辑吗？'
              : `确定要${forceTakeover ? '强制接管' : '请求接管'}编辑权限吗？`}
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
            className={styles.btnPrimary}
            onClick={() => onConfirm(isSameUser ? true : forceTakeover)}
            type="button"
          >
            {isSameUser ? '确认接管' : (forceTakeover ? '强制接管' : '请求接管')}
          </button>
        </div>
      </div>
    </div>
  );
};

export default TakeoverConfirmDialog;
