import React, { useState, useEffect } from 'react';
import styles from './TakeoverConfirmDialog.module.css';

interface TakeoverWaitingDialogProps {
  targetUserName: string;
  isSameUser: boolean;
  onCancel: () => void;
}

/**
 * 接管方等待对话框
 * 当发起接管请求后，等待对方响应时显示
 */
const TakeoverWaitingDialog: React.FC<TakeoverWaitingDialogProps> = ({
  targetUserName,
  isSameUser,
  onCancel,
}) => {
  const [countdown, setCountdown] = useState(30);

  useEffect(() => {
    // 30秒倒计时
    const timer = setInterval(() => {
      setCountdown((prev) => {
        if (prev <= 1) {
          return 0;
        }
        return prev - 1;
      });
    }, 1000);

    return () => clearInterval(timer);
  }, []);

  return (
    <div className={styles.overlay}>
      <div className={styles.dialog}>
        <div className={styles.header}>
          <h3 className={styles.title}>等待确认</h3>
        </div>

        <div className={styles.content}>
          <div className={styles.infoBox}>
            <p className={styles.mainText}>
              {isSameUser 
                ? '正在等待您的另一个窗口确认接管...' 
                : `正在等待 ${targetUserName} 确认接管...`}
            </p>
          </div>

          <div className={styles.countdownBox}>
            <p className={styles.countdownText}>
              剩余时间: <strong className={styles.countdownNumber}>{countdown}</strong> 秒
            </p>
          </div>

          <div className={styles.loadingIndicator}>
            <div className={styles.spinner}></div>
            <p>等待对方响应...</p>
          </div>
        </div>

        <div className={styles.actions}>
          <button
            className={styles.btnSecondary}
            onClick={onCancel}
            type="button"
          >
            取消请求
          </button>
        </div>
      </div>
    </div>
  );
};

export default TakeoverWaitingDialog;
