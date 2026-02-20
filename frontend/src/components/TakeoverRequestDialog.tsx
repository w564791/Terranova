import React, { useState, useEffect, useRef } from 'react';
import styles from './TakeoverConfirmDialog.module.css';

interface TakeoverRequest {
  id: number;
  requester_name: string;
  requester_user_id: string;
  is_same_user: boolean;
  expires_at: string;
}

interface TakeoverRequestDialogProps {
  request: TakeoverRequest;
  onApprove: () => void;
  onReject: () => void;
}

/**
 * 被接管方确认对话框
 * 当其他用户（或同一用户的另一个窗口）请求接管编辑时显示
 */
const TakeoverRequestDialog: React.FC<TakeoverRequestDialogProps> = ({
  request,
  onApprove,
  onReject,
}) => {
  // 使用服务器过期时间计算初始倒计时
  const [countdown, setCountdown] = useState(() => {
    const expiresAt = new Date(request.expires_at).getTime();
    const remaining = Math.max(0, Math.floor((expiresAt - Date.now()) / 1000));
    return remaining > 0 ? remaining : 30; // 如果解析失败，默认30秒
  });
  
  // 使用ref存储onApprove，避免闭包问题
  const onApproveRef = useRef(onApprove);
  onApproveRef.current = onApprove;
  
  // 标记是否已经触发过自动同意
  const hasAutoApprovedRef = useRef(false);

  useEffect(() => {
    const expiresAt = new Date(request.expires_at).getTime();
    
    const timer = setInterval(() => {
      const remaining = Math.max(0, Math.floor((expiresAt - Date.now()) / 1000));
      setCountdown(remaining);
      
      if (remaining <= 0 && !hasAutoApprovedRef.current) {
        hasAutoApprovedRef.current = true;
        clearInterval(timer);
        // 超时自动同意接管
        onApproveRef.current();
      }
    }, 1000);

    return () => clearInterval(timer);
  }, [request.expires_at]);

  return (
    <div className={styles.overlay}>
      <div className={styles.dialog}>
        <div className={styles.header}>
          <h3 className={styles.title}>接管编辑请求</h3>
        </div>

        <div className={styles.content}>
          <div className={styles.infoBox}>
            <p className={styles.mainText}>
              {request.is_same_user 
                ? '您在另一个窗口尝试接管此编辑会话' 
                : `用户 ${request.requester_name} 尝试接管此编辑会话`}
            </p>
          </div>

          <div className={styles.warningBox}>
            <span className={styles.warningIcon}></span>
            <p className={styles.warningText}>
              如果同意接管，当前窗口将无法继续编辑。
              <br />
              您的未保存内容将被保留为草稿。
            </p>
          </div>

          <div className={styles.countdownBox}>
            <p className={styles.countdownText}>
              请在 <strong className={styles.countdownNumber}>{countdown}</strong> 秒内做出决定
            </p>
            <p className={styles.countdownHint}>
              超时将自动同意接管请求
            </p>
          </div>
        </div>

        <div className={styles.actions}>
          <button
            className={styles.btnSecondary}
            onClick={onReject}
            type="button"
          >
            拒绝接管
          </button>
          <button
            className={styles.btnPrimary}
            onClick={onApprove}
            type="button"
          >
            同意接管
          </button>
        </div>
      </div>
    </div>
  );
};

export default TakeoverRequestDialog;
