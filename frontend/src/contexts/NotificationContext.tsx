import React, { createContext, useContext } from 'react';
import { useNotification } from '../hooks/useNotification';
import NotificationContainer from '../components/NotificationContainer';

const NotificationContext = createContext<ReturnType<typeof useNotification> | null>(null);

export const NotificationProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const notification = useNotification();

  return (
    <NotificationContext.Provider value={notification}>
      {children}
      <NotificationContainer
        notifications={notification.notifications}
        onRemove={notification.removeNotification}
      />
    </NotificationContext.Provider>
  );
};

export const useNotificationContext = () => {
  const context = useContext(NotificationContext);
  if (!context) {
    throw new Error('useNotificationContext must be used within NotificationProvider');
  }
  return context;
};