/**
 * IAM context left nav (vertical). Same visual language as WorkspaceSidebar.
 * Mounted only via IAMLayout → ContextShell.
 */
import React from 'react';
import { Link, useLocation, useNavigate } from 'react-router-dom';
import { useSelector } from 'react-redux';
import type { RootState } from '../store';
import styles from './WorkspaceSidebar.module.css';

export interface IAMSidebarProps {
  mobileSidebarOpen?: boolean;
  onMobileSidebarClose?: () => void;
}

const navItems = [
  { path: '/iam/organizations', label: 'Organizations' },
  { path: '/iam/projects', label: 'Projects' },
  { path: '/iam/users', label: 'Users' },
  { path: '/iam/teams', label: 'Teams' },
  { path: '/iam/applications', label: 'Applications' },
  { path: '/iam/permissions', label: 'Permissions' },
  { path: '/iam/roles', label: 'Roles' },
  { path: '/iam/audit', label: 'Audit Logs' },
];

function isActive(pathname: string, itemPath: string): boolean {
  if (itemPath === '/iam/teams') {
    return pathname === itemPath || pathname.startsWith(`${itemPath}/`);
  }
  if (itemPath === '/iam/permissions') {
    return pathname === itemPath || pathname.startsWith(`${itemPath}/`);
  }
  return pathname === itemPath || pathname.startsWith(`${itemPath}/`);
}

const IAMSidebar: React.FC<IAMSidebarProps> = ({
  mobileSidebarOpen = false,
  onMobileSidebarClose,
}) => {
  const navigate = useNavigate();
  const location = useLocation();
  const { user } = useSelector((state: RootState) => state.auth);
  const visibleNavItems = navItems.filter(
    (item) => item.path !== '/iam/users' || user?.is_system_admin,
  );

  return (
    <aside className={`${styles.sidebar} ${mobileSidebarOpen ? styles.sidebarMobileOpen : ''}`}>
      <div className={styles.sidebarHeader}>
        <button
          type="button"
          onClick={() => navigate('/')}
          className={styles.backButton}
        >
          ← Back to Main
        </button>
        <h1 className={styles.workspaceTitle}>IAM System</h1>
      </div>

      <nav className={styles.nav} aria-label="IAM">
        {visibleNavItems.map((item) => (
          <Link
            key={item.path}
            to={item.path}
            className={`${styles.navItem} ${
              isActive(location.pathname, item.path) ? styles.navItemActive : ''
            }`}
            onClick={() => onMobileSidebarClose?.()}
          >
            <span className={styles.navLabel}>{item.label}</span>
          </Link>
        ))}
      </nav>
    </aside>
  );
};

export default IAMSidebar;
