import React, { createContext, useContext, useMemo } from 'react';
import { useSimpleToast } from '../hooks/useSimpleToast';
import { FEATURES } from '../config/features';
import SimpleToast from '../components/SimpleToast';

interface ToastContextType {
  showToast: (message: string, type: 'success' | 'error' | 'warning' | 'info') => void;
  success: (message: string) => void;
  error: (message: string) => void;
  warning: (message: string) => void;
  info: (message: string) => void;
}

const ToastContext = createContext<ToastContextType | undefined>(undefined);

export const ToastProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const simpleToast = useSimpleToast();

  // 使用 useMemo 缓存 contextValue，避免每次渲染都创建新对象导致子组件重新渲染
  // 关键：不依赖 simpleToast 对象本身，而是依赖其方法
  const contextValue: ToastContextType = useMemo(() => ({
    showToast: (message: string, type: 'success' | 'error' | 'warning' | 'info') => {
      console.log('🔔 showToast called:', { message, type, featureEnabled: FEATURES.TOAST_NOTIFICATIONS });
      if (FEATURES.TOAST_NOTIFICATIONS) {
        console.log('🔔 Calling simpleToast[type]:', type);
        simpleToast[type](message);
      } else {
        alert(message);
      }
    },
    success: (message: string) => {
      if (FEATURES.TOAST_NOTIFICATIONS) {
        simpleToast.success(message);
      } else {
        alert(message);
      }
    },
    error: (message: string) => {
      if (FEATURES.TOAST_NOTIFICATIONS) {
        simpleToast.error(message);
      } else {
        alert(message);
      }
    },
    warning: (message: string) => {
      if (FEATURES.TOAST_NOTIFICATIONS) {
        simpleToast.warning(message);
      } else {
        alert(message);
      }
    },
    info: (message: string) => {
      if (FEATURES.TOAST_NOTIFICATIONS) {
        simpleToast.info(message);
      } else {
        alert(message);
      }
    }
  }), [simpleToast.success, simpleToast.error, simpleToast.warning, simpleToast.info]);

  return (
    <ToastContext.Provider value={contextValue}>
      {children}
      {FEATURES.TOAST_NOTIFICATIONS && (
        <SimpleToast
          message={simpleToast.toast.message}
          type={simpleToast.toast.type}
          isVisible={simpleToast.toast.isVisible}
          onClose={simpleToast.hideToast}
        />
      )}
    </ToastContext.Provider>
  );
};

export const useToast = (): ToastContextType => {
  const context = useContext(ToastContext);
  if (!context) {
    throw new Error('useToast must be used within a ToastProvider');
  }
  return context;
};
