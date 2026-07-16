import React, { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useSelector, useDispatch } from 'react-redux';
import type { RootState } from '../store';
import { logout } from '../store/slices/authSlice';
import { authService } from '../services/auth';
import MotivationalQuote from './MotivationalQuote';
import { useUIVersion } from '../hooks/useUIVersion';
import styles from './TopBar.module.css';

interface TopBarProps {
  /** Extra class on the header (e.g. Layout mobile hamburger offset) */
  className?: string;
}

const TopBar: React.FC<TopBarProps> = ({ className }) => {
  const navigate = useNavigate();
  const dispatch = useDispatch();
  const { user } = useSelector((state: RootState) => state.auth);
  const [showUserMenu, setShowUserMenu] = useState(false);
  const { isV3, setVersion } = useUIVersion();

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
    <header className={`${styles.header}${className ? ` ${className}` : ''}`}>
      <MotivationalQuote username={user?.username} />

      <div className={styles.actions}>
        <button
          type="button"
          className={`${styles.uiToggle} ${isV3 ? styles.uiToggleActive : ''}`}
          onClick={() => setVersion(isV3 ? 'v2' : 'v3')}
          title={isV3 ? '切换到经典 UI (v2)' : '切换到新版 UI (v3)'}
        >
          UI {isV3 ? 'v3' : 'v2'}
        </button>
        <div
          className={styles.userMenu}
          onClick={() => setShowUserMenu(!showUserMenu)}
          onKeyDown={(e) => {
            if (e.key === 'Enter' || e.key === ' ') {
              e.preventDefault();
              setShowUserMenu(!showUserMenu);
            }
          }}
          role="button"
          tabIndex={0}
        >
          <div className={styles.avatar}>
            {user?.username?.charAt(0).toUpperCase()}
          </div>
          <span className={styles.username}>{user?.username}</span>

          {showUserMenu && (
            <div className={styles.dropdown}>
              <button
                type="button"
                className={styles.dropdownItem}
                onClick={() => {
                  setShowUserMenu(false);
                  navigate('/settings');
                }}
              >
                Settings
              </button>
              <button type="button" className={styles.dropdownItem} onClick={handleLogout}>
                Logout
              </button>
            </div>
          )}
        </div>
      </div>
    </header>
  );
};

export default TopBar;
