import React from 'react';

interface SimpleToastProps {
  message: string;
  type: 'success' | 'error' | 'warning' | 'info';
  isVisible: boolean;
  onClose: () => void;
}

const SimpleToast: React.FC<SimpleToastProps> = ({ message, type, isVisible, onClose }) => {
  if (!isVisible) return null;

  const getStyles = () => {
    const baseStyle = {
      position: 'fixed' as const,
      bottom: '24px',
      left: '24px',
      padding: '12px 16px',
      borderRadius: '8px',
      color: 'white',
      fontSize: '14px',
      zIndex: 1000,
      display: 'flex',
      alignItems: 'center',
      gap: '8px',
      width: '320px',
      minHeight: '108px',
      maxWidth: '320px',
      boxShadow: '0 4px 12px rgba(0, 0, 0, 0.15)',
      wordWrap: 'break-word' as const
    };

    const typeStyles = {
      success: { backgroundColor: '#10B981' },
      error: { backgroundColor: '#EF4444' },
      warning: { backgroundColor: '#F59E0B' },
      info: { backgroundColor: '#3B82F6' }
    };

    return { ...baseStyle, ...typeStyles[type] };
  };

  return (
    <div style={getStyles()}>
      <span>{message}</span>
      <button 
        onClick={onClose}
        style={{
          background: 'none',
          border: 'none',
          color: 'white',
          cursor: 'pointer',
          fontSize: '16px',
          marginLeft: 'auto'
        }}
      >
        ×
      </button>
    </div>
  );
};

export default SimpleToast;
