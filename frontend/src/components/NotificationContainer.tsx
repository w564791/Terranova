import React, { useState, useEffect } from 'react';
import type { Notification } from '../hooks/useNotification';
import styles from './NotificationContainer.module.css';

interface NotificationItemProps {
  notification: Notification;
  onRemove: (id: string) => void;
}

const NotificationItem: React.FC<NotificationItemProps> = ({ notification, onRemove }) => {
  const [isHovered, setIsHovered] = useState(false);
  const [, setTimeLeft] = useState(5);

  useEffect(() => {
    if (isHovered) return;

    const timer = setInterval(() => {
      setTimeLeft(prev => {
        if (prev <= 1) {
          onRemove(notification.id);
          return 0;
        }
        return prev - 1;
      });
    }, 1000);

    return () => clearInterval(timer);
  }, [isHovered, notification.id, onRemove]);

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(notification.message);
    } catch (err) {
      console.error('复制失败:', err);
    }
  };

  return (
    <div
      className={`${styles.notification} ${styles[notification.type]}`}
      onMouseEnter={() => setIsHovered(true)}
      onMouseLeave={() => setIsHovered(false)}
      onClick={handleCopy}
      title="点击复制"
    >
      <div className={styles.message}>{notification.message}</div>
      {isHovered && (
        <div className={styles.copyHint}>点击复制</div>
      )}
    </div>
  );
};

interface NotificationContainerProps {
  notifications: Notification[];
  onRemove: (id: string) => void;
}

const NotificationContainer: React.FC<NotificationContainerProps> = ({ 
  notifications, 
  onRemove 
}) => {
  return (
    <div className={styles.container}>
      {notifications.map(notification => (
        <NotificationItem
          key={notification.id}
          notification={notification}
          onRemove={onRemove}
        />
      ))}
    </div>
  );
};

export default NotificationContainer;