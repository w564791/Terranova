import React, { useState } from 'react';
import { Outlet, useNavigate, useLocation } from 'react-router-dom';
import TopBar from './TopBar';
import styles from './IAMLayout.module.css';

const IAMLayout: React.FC = () => {
  const navigate = useNavigate();
  const location = useLocation();
  const [mobileSidebarOpen, setMobileSidebarOpen] = useState(false);

  // Navigation menu items
  const navItems = [
    { path: '/iam/organizations', label: 'Organizations', icon: '' },
    { path: '/iam/projects', label: 'Projects', icon: '' },
    { path: '/iam/users', label: 'Users', icon: '' },
    { path: '/iam/teams', label: 'Teams', icon: '' },
    { path: '/iam/applications', label: 'Applications', icon: '' },
    { path: '/iam/permissions', label: 'Permissions', icon: '' },
    { path: '/iam/roles', label: 'Roles', icon: '' },
    { path: '/iam/audit', label: 'Audit Logs', icon: '' },
  ];

  const handleNavClick = (path: string) => {
    navigate(path);
    setMobileSidebarOpen(false);
  };

  const handleBackToMain = () => {
    navigate('/');
  };

  // 检查当前路径是否匹配（支持子路径）
  const isActive = (path: string) => {
    if (path === '/iam/teams') {
      // 团队管理特殊处理：/iam/teams 和 /iam/teams/:id 都算激活
      return location.pathname === path || location.pathname.startsWith(path + '/');
    }
    return location.pathname === path;
  };

  return (
    <div className={styles.iamLayout}>
      {/* 移动端汉堡菜单按钮 */}
      <button 
        className={styles.mobileSidebarButton}
        onClick={() => setMobileSidebarOpen(true)}
        aria-label="Open menu"
      >
        ☰
      </button>

      {/* 移动端遮罩层 */}
      {mobileSidebarOpen && (
        <div 
          className={styles.mobileSidebarOverlay}
          onClick={() => setMobileSidebarOpen(false)}
        />
      )}

      {/* 左侧导航栏 */}
      <aside className={`${styles.iamSidebar} ${mobileSidebarOpen ? styles.sidebarMobileOpen : ''}`}>
        <div className={styles.iamHeader}>
          <button onClick={handleBackToMain} className={styles.backButton}>
            ← Back to Main
          </button>
          <h1 className={styles.iamTitle}>IAM System</h1>
        </div>

        {/* 导航菜单 */}
        <nav className={styles.iamNav}>
          {navItems.map((item) => (
            <button
              key={item.path}
              className={`${styles.navItem} ${
                isActive(item.path) ? styles.navItemActive : ''
              }`}
              onClick={() => handleNavClick(item.path)}
            >
              <span className={styles.navLabel}>{item.label}</span>
            </button>
          ))}
        </nav>

      </aside>

      {/* 右侧内容区 */}
      <main className={styles.iamMain}>
        <TopBar title="IAM" />
        
        <div className={styles.iamContent}>
          <Outlet />
        </div>
      </main>
    </div>
  );
};

export default IAMLayout;
