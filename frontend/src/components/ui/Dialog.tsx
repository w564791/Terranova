/**
 * Unified Dialog shell — portal + rounded card
 *
 * <Dialog open onClose={...} title="标题" footer={...}>
 *   body
 * </Dialog>
 */
import React, { useEffect, useId, useCallback } from 'react';
import { createPortal } from 'react-dom';

export type DialogTone = 'default' | 'info' | 'warning' | 'danger';
export type DialogSize = 'sm' | 'md' | 'lg' | 'xl';

export interface DialogProps {
  open: boolean;
  onClose: () => void;
  title?: React.ReactNode;
  children?: React.ReactNode;
  footer?: React.ReactNode;
  /** Visual tone for header accent */
  tone?: DialogTone;
  size?: DialogSize;
  /** Close when clicking overlay (default true) */
  closeOnOverlay?: boolean;
  /** Close on Escape (default true) */
  closeOnEsc?: boolean;
  /** Show × button (default false for confirm dialogs) */
  showClose?: boolean;
  className?: string;
  bodyClassName?: string;
  /** aria-labelledby override */
  'aria-label'?: string;
}

export const Dialog: React.FC<DialogProps> = ({
  open,
  onClose,
  title,
  children,
  footer,
  tone = 'default',
  size = 'md',
  closeOnOverlay = true,
  closeOnEsc = true,
  showClose = false,
  className,
  bodyClassName,
  'aria-label': ariaLabel,
}) => {
  const titleId = useId();

  const handleKeyDown = useCallback(
    (e: KeyboardEvent) => {
      if (closeOnEsc && e.key === 'Escape') {
        e.stopPropagation();
        onClose();
      }
    },
    [closeOnEsc, onClose]
  );

  useEffect(() => {
    if (!open) return;
    document.addEventListener('keydown', handleKeyDown);
    const prev = document.body.style.overflow;
    document.body.style.overflow = 'hidden';
    return () => {
      document.removeEventListener('keydown', handleKeyDown);
      document.body.style.overflow = prev;
    };
  }, [open, handleKeyDown]);

  if (!open) return null;
  if (typeof document === 'undefined') return null;

  const toneClass = tone !== 'default' ? ` tn-dialog--${tone}` : '';
  const sizeClass = ` tn-dialog--${size}`;

  return createPortal(
    <div
      className="tn-overlay"
      role="presentation"
      onClick={closeOnOverlay ? onClose : undefined}
    >
      <div
        className={`tn-dialog${sizeClass}${toneClass}${className ? ` ${className}` : ''}`}
        role="dialog"
        aria-modal="true"
        aria-labelledby={title ? titleId : undefined}
        aria-label={!title ? ariaLabel : undefined}
        onClick={(e) => e.stopPropagation()}
      >
        {(title != null || showClose) && (
          <div className="tn-dialog__header">
            {title != null ? (
              <h3 className="tn-dialog__title" id={titleId}>
                {title}
              </h3>
            ) : (
              <span />
            )}
            {showClose && (
              <button
                type="button"
                className="tn-dialog__close"
                onClick={onClose}
                aria-label="关闭"
              >
                ×
              </button>
            )}
          </div>
        )}
        {children != null && (
          <div className={`tn-dialog__body${bodyClassName ? ` ${bodyClassName}` : ''}`}>
            {children}
          </div>
        )}
        {footer != null && <div className="tn-dialog__footer">{footer}</div>}
      </div>
    </div>,
    document.body
  );
};

export default Dialog;
