import React, { useEffect } from 'react';
import type { Notification } from '../hooks/useNotification';
import styles from './NotificationContainer.module.css';

interface NotificationItemProps {
  notification: Notification;
  onRemove: (id: string) => void;
}

const NotificationItem: React.FC<NotificationItemProps> = ({ notification, onRemove }) => {
  useEffect(() => {
    const timer = setTimeout(() => {
      onRemove(notification.id);
    }, 4000);
    return () => clearTimeout(timer);
  }, [notification.id, onRemove]);

  return (
    <div className={`${styles.notification} ${styles[notification.type]}`}>
      <span className={styles.message}>{notification.message}</span>
      <button className={styles.closeButton} onClick={() => onRemove(notification.id)}>×</button>
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
  if (notifications.length === 0) return null;

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
