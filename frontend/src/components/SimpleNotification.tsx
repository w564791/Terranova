import React, { useState, useEffect } from 'react';

interface NotificationProps {
  message: string;
  type: 'success' | 'error' | 'warning' | 'info';
  onClose: () => void;
}

const SimpleNotification: React.FC<NotificationProps> = ({ message, type, onClose }) => {
  const [isHovered, setIsHovered] = useState(false);

  useEffect(() => {
    if (isHovered) return;

    const timer = setTimeout(() => {
      onClose();
    }, 5000);

    return () => clearTimeout(timer);
  }, [isHovered, onClose]);

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(message);
    } catch (err) {
      console.error('复制失败:', err);
    }
  };

  const getBackgroundColor = () => {
    switch (type) {
      case 'success': return '#22c55e';
      case 'error': return '#ef4444';
      case 'warning': return '#f59e0b';
      case 'info': return '#3b82f6';
      default: return '#3b82f6';
    }
  };

  return (
    <div
      style={{
        position: 'fixed',
        bottom: '20px',
        left: '20px',
        minWidth: '300px',
        maxWidth: '400px',
        padding: '16px',
        borderRadius: '8px',
        backgroundColor: getBackgroundColor(),
        color: 'white',
        fontFamily: 'var(--font-primary)',
        fontSize: 'var(--font-size-sm)',
        fontWeight: 'var(--font-weight-normal)',
        lineHeight: 'var(--line-height-normal)',
        boxShadow: '0 4px 12px rgba(0, 0, 0, 0.15)',
        cursor: 'pointer',
        transition: 'all 0.3s ease',
        zIndex: 9999,
        transform: isHovered ? 'translateY(-2px)' : 'translateY(0)',
      }}
      onMouseEnter={() => setIsHovered(true)}
      onMouseLeave={() => setIsHovered(false)}
      onClick={handleCopy}
      title="点击复制"
    >
      {message}
      {isHovered && (
        <div
          style={{
            position: 'absolute',
            top: '-30px',
            right: '0',
            background: 'rgba(0, 0, 0, 0.8)',
            color: 'white',
            padding: '4px 8px',
            borderRadius: '4px',
            fontSize: '12px',
            whiteSpace: 'nowrap',
          }}
        >
          点击复制
        </div>
      )}
    </div>
  );
};

export default SimpleNotification;