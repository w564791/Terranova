import React, { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useSelector, useDispatch } from 'react-redux';
import type { RootState } from '../store';
import { logout } from '../store/slices/authSlice';
import { authService } from '../services/auth';
import MotivationalQuote from './MotivationalQuote';
import styles from './TopBar.module.css';

interface TopBarProps {
  title?: string;
}

const TopBar: React.FC<TopBarProps> = ({ title = 'IaC 平台' }) => {
  const navigate = useNavigate();
  const dispatch = useDispatch();
  const { user } = useSelector((state: RootState) => state.auth);
  const [showUserMenu, setShowUserMenu] = useState(false);

  const handleLogout = async () => {
    try {
      await authService.logout();
    } catch (error) {
      console.error('Logout error:', error);
    } finally {
      dispatch(logout());
      navigate('/login');
    }
    setShowUserMenu(false);
  };

  return (
    <header className={styles.header}>
      <h2 className={styles.headerTitle}>{title}</h2>
      
      <div style={{ display: 'flex', alignItems: 'center', gap: '16px' }}>
        <MotivationalQuote username={user?.username} />
        
        <div className={styles.userMenu} onClick={() => setShowUserMenu(!showUserMenu)}>
          <div className={styles.avatar}>
            {user?.username?.charAt(0).toUpperCase()}
          </div>
          <span className={styles.username}>{user?.username}</span>
          
          {showUserMenu && (
            <div className={styles.dropdown}>
              <button
                className={styles.dropdownItem}
                onClick={() => {
                  setShowUserMenu(false);
                  navigate('/settings');
                }}
              >
                个人设置
              </button>
              <button
                className={styles.dropdownItem}
                onClick={handleLogout}
              >
                退出登录
              </button>
            </div>
          )}
        </div>
      </div>
    </header>
  );
};

export default TopBar;
