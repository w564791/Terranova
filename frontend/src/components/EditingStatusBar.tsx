import React from 'react';
import type { EditorInfo } from '../services/resourceEditing';
import styles from './EditingStatusBar.module.css';

interface EditingStatusBarProps {
  otherEditors: EditorInfo[];
  hasVersionConflict: boolean;
  isDisabled: boolean;
  onShowDetails?: () => void;
}

const EditingStatusBar: React.FC<EditingStatusBarProps> = ({
  otherEditors,
  hasVersionConflict,
  isDisabled,
  onShowDetails,
}) => {
  // 处理null情况
  const editors = otherEditors || [];
  
  const getStatusColor = (): 'green' | 'yellow' | 'orange' | 'red' => {
    if (isDisabled) return 'red';
    if (hasVersionConflict) return 'orange';
    if (editors.some(e => e.is_same_user)) return 'yellow';
    if (editors.length > 0) return 'red';
    return 'green';
  };

  const getStatusText = (): string => {
    if (isDisabled) return '编辑已被接管';
    if (hasVersionConflict) return '资源版本已更新，无法提交';
    
    const sameUserEditors = editors.filter(e => e.is_same_user);
    if (sameUserEditors.length > 0) {
      return '您在其他窗口正在编辑';
    }
    
    if (editors.length > 0) {
      return `${editors[0].user_name}正在编辑`;
    }
    
    return '可以安全编辑';
  };

  const getStatusIcon = (): string => {
    const color = getStatusColor();
    switch (color) {
      case 'green':
        return '✓';
      case 'yellow':
        return '⚠';
      case 'orange':
        return '⚠';
      case 'red':
        return '✕';
      default:
        return '•';
    }
  };

  const statusColor = getStatusColor();

  return (
    <div className={`${styles.statusBar} ${styles[`status-${statusColor}`]}`}>
      <div className={styles.statusContent}>
        <div className={styles.statusIndicator}>
          <span className={styles.statusIcon}>{getStatusIcon()}</span>
          <span className={styles.statusText}>{getStatusText()}</span>
        </div>
        
        {editors.length > 0 && (
          <div className={styles.statusActions}>
            <button 
              className={styles.detailsButton}
              onClick={onShowDetails}
              type="button"
            >
              查看详情 ({otherEditors.length})
            </button>
          </div>
        )}
      </div>
      
      {hasVersionConflict && (
        <div className={styles.warningMessage}>
           资源已被其他用户修改，请刷新页面查看最新版本
        </div>
      )}
    </div>
  );
};

export default EditingStatusBar;
