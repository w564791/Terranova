import React from 'react';
import styles from './ConfirmDialog.module.css';

interface Props {
  isOpen: boolean;
  title: string;
  message?: string;
  confirmText?: string;
  cancelText?: string;
  onConfirm: () => void;
  onCancel: () => void;
  type?: 'info' | 'warning' | 'danger';
  loading?: boolean;
  confirmDisabled?: boolean;
  children?: React.ReactNode;
}

const ConfirmDialog: React.FC<Props> = ({
  isOpen,
  title,
  message,
  confirmText = '确认',
  cancelText = '取消',
  onConfirm,
  onCancel,
  type = 'info',
  loading = false,
  confirmDisabled = false,
  children
}) => {
  if (!isOpen) return null;

  return (
    <div className={styles.overlay} onClick={onCancel}>
      <div className={styles.dialog} onClick={(e) => e.stopPropagation()}>
        <div className={`${styles.header} ${type !== 'info' ? styles[type] : ''}`}>
          <h3 className={`${styles.title} ${type !== 'info' ? styles[type] : ''}`}>{title}</h3>
        </div>
        
        <div className={styles.body}>
          {message && <p className={styles.message}>{message}</p>}
          {children}
        </div>
        
        <div className={styles.footer}>
          <button
            onClick={onCancel}
            className={styles.btnCancel}
          >
            {cancelText}
          </button>
          <button
            onClick={onConfirm}
            className={`${styles.btnConfirm} ${styles[type]}`}
            disabled={loading || confirmDisabled}
          >
            {loading ? '处理中...' : confirmText}
          </button>
        </div>
      </div>
    </div>
  );
};

export default ConfirmDialog;
