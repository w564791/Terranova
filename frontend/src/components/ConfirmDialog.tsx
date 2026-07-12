/**
 * ConfirmDialog — 统一警示/确认弹窗
 * 基于 Dialog shell + 设计系统按钮（圆角矩形卡片）
 */
import React from 'react';
import { Dialog, type DialogTone } from './ui/Dialog';

export type ConfirmType = 'info' | 'warning' | 'danger';

export interface ConfirmDialogProps {
  isOpen: boolean;
  title: string;
  message?: string;
  confirmText?: string;
  cancelText?: string;
  onConfirm: () => void;
  onCancel: () => void;
  /** @deprecated use onCancel — kept for CreateWorkspaceDialog compatibility */
  onClose?: () => void;
  type?: ConfirmType;
  loading?: boolean;
  confirmDisabled?: boolean;
  children?: React.ReactNode;
  size?: 'sm' | 'md' | 'lg' | 'xl';
  showClose?: boolean;
}

const ConfirmDialog: React.FC<ConfirmDialogProps> = ({
  isOpen,
  title,
  message,
  confirmText = '确认',
  cancelText = '取消',
  onConfirm,
  onCancel,
  onClose,
  type = 'info',
  loading = false,
  confirmDisabled = false,
  children,
  size = 'md',
  showClose = false,
}) => {
  const handleDismiss = () => {
    if (loading) return;
    onCancel();
    onClose?.();
  };

  const tone: DialogTone =
    type === 'danger' ? 'danger' : type === 'warning' ? 'warning' : 'info';

  // danger → solid red；其余 → solid brand（显式 color 防止被旧 .btn.red 覆盖）
  const confirmBtnClass =
    type === 'danger' ? 'btn solid red' : 'btn solid brand';

  return (
    <Dialog
      open={isOpen}
      onClose={handleDismiss}
      title={title}
      tone={tone}
      size={size}
      showClose={showClose}
      closeOnOverlay={!loading}
      closeOnEsc={!loading}
      footer={
        <>
          <button
            type="button"
            className="btn"
            onClick={handleDismiss}
            disabled={loading}
          >
            {cancelText}
          </button>
          <button
            type="button"
            className={`${confirmBtnClass}${loading ? ' is-loading' : ''}`}
            style={{ color: '#fff' }}
            onClick={onConfirm}
            disabled={loading || confirmDisabled}
          >
            {loading ? '处理中...' : confirmText}
          </button>
        </>
      }
    >
      {message && <p className="tn-dialog__message">{message}</p>}
      {children}
    </Dialog>
  );
};

export default ConfirmDialog;
