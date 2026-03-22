import React, { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useSelector, useDispatch } from 'react-redux';
import type { RootState } from '../store';
import { logout } from '../store/slices/authSlice';
import { authService } from '../services/auth';
import MotivationalQuote from './MotivationalQuote';
import styles from './TopBar.module.css';

const TopBar: React.FC = () => {
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
    </header>
  );
};

export default TopBar;
