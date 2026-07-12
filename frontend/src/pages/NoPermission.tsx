import React, { useState } from 'react';
import { useSelector } from 'react-redux';
import type { RootState } from '../store';
import styles from './NoPermission.module.css';

const NoPermission: React.FC = () => {
  const { user } = useSelector((state: RootState) => state.auth);
  const [copied, setCopied] = useState(false);

  const handleCopyUsername = () => {
    if (user?.username) {
      navigator.clipboard.writeText(user.username).then(() => {
        setCopied(true);
        setTimeout(() => setCopied(false), 2000);
      });
    }
  };

  return (
    <div className={styles.container}>
      <div className={styles.content}>
        <div className={styles.icon}></div>
        <h1 className={styles.title}>您好，{user?.username || '用户'}</h1>
        <p className={styles.message}>
          您尚未加入任何组织，暂时无法访问系统功能。
        </p>
        
        <div className={styles.userInfo}>
          <div className={styles.userInfoLabel}>您的用户名</div>
          <div className={styles.userInfoValue}>
            <span className={styles.username}>{user?.username || 'N/A'}</span>
            <button 
              className={styles.copyButton}
              onClick={handleCopyUsername}
              title="复制用户名"
            >
              {copied ? '已复制' : '复制'}
            </button>
          </div>
          <div className={styles.userInfoHint}>
            请将此用户名提供给管理员以便授予权限
          </div>
        </div>

        <p className={styles.hint}>
          请联系系统管理员为您授予相应的权限。
        </p>
        
        <div className={styles.info}>
          <h3>需要帮助？</h3>
          <ul>
            <li>将您的用户名（{user?.username}）提供给组织管理员</li>
            <li>确认您的账号已被激活</li>
            <li>等待管理员为您分配权限</li>
            <li>权限授予后，刷新页面即可访问系统</li>
          </ul>
        </div>
      </div>
    </div>
  );
};

export default NoPermission;
