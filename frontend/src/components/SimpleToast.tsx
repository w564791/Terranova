import React from 'react';
import { statusColor, type StatusType } from '../styles/tokens';

interface SimpleToastProps {
  message: string;
  type: 'success' | 'error' | 'warning' | 'info';
  isVisible: boolean;
  onClose: () => void;
}

const SimpleToast: React.FC<SimpleToastProps> = ({ message, type, isVisible, onClose }) => {
  if (!isVisible) return null;

  return (
    <div
      className={`tn-notice tn-notice--${type}`}
      style={{
        position: 'fixed',
        bottom: 24,
        left: 24,
        zIndex: 1000,
        // ensure background even if global class missing in tests
        backgroundColor: statusColor(type as StatusType),
      }}
    >
      <span style={{ flex: 1, wordWrap: 'break-word' }}>{message}</span>
      <button type="button" className="tn-notice__close" onClick={onClose} aria-label="关闭">
        ×
      </button>
    </div>
  );
};

export default SimpleToast;
