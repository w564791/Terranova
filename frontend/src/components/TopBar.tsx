import React, { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useSelector, useDispatch } from 'react-redux';
import type { RootState } from '../store';
import { logout } from '../store/slices/authSlice';
import { authService } from '../services/auth';
import MotivationalQuote from './MotivationalQuote';
import { useUIVersion } from '../hooks/useUIVersion';
import styles from './TopBar.module.css';

const TopBar: React.FC = () => {
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
    <header className={styles.header}>
      <MotivationalQuote username={user?.username} />

      <div style={{ display: 'flex', alignItems: 'center', gap: '16px' }}>
        <button
          onClick={() => setVersion(isV3 ? 'v2' : 'v3')}
          style={{
            padding: '4px 12px',
            borderRadius: '6px',
            border: '1px solid',
            borderColor: isV3 ? 'var(--brand-300)' : 'var(--line)',
            background: isV3 ? 'var(--brand-soft)' : 'var(--surface)',
            color: isV3 ? 'var(--brand-ink)' : 'var(--ink-2)',
            fontSize: '12px',
            fontWeight: 500,
            cursor: 'pointer',
            transition: 'all 0.15s',
            lineHeight: '1.4',
          }}
          title={isV3 ? '切换到经典 UI (v2)' : '切换到新版 UI (v3)'}
        >
          UI {isV3 ? 'v3' : 'v2'}
        </button>
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
                Settings
              </button>
              <button
                className={styles.dropdownItem}
                onClick={handleLogout}
              >
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
