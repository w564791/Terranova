import React, { useState, useEffect } from 'react';
import { statusColor, type StatusType } from '../styles/tokens';

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

  return (
    <div
      className={`tn-notice tn-notice--${type}`}
      style={{
        position: 'fixed',
        bottom: 20,
        left: 20,
        zIndex: 9999,
        backgroundColor: statusColor(type as StatusType),
        cursor: 'pointer',
        transition: 'transform 0.3s ease',
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
