import React, { useState } from 'react';
import { Outlet, useNavigate, useLocation, Link } from 'react-router-dom';
import { useSelector, useDispatch } from 'react-redux';
import type { RootState } from '../store';
import { logout } from '../store/slices/authSlice';
import { authService } from '../services/auth';
import MotivationalQuote from './MotivationalQuote';
import NoPermission from '../pages/NoPermission';
import styles from './Layout.module.css';

const Layout: React.FC = () => {
  const navigate = useNavigate();
  const location = useLocation();
  const dispatch = useDispatch();
  const { user, isAuthenticated } = useSelector((state: RootState) => state.auth);
  const [showUserMenu, setShowUserMenu] = useState(false);
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false);
  const [mobileMenuOpen, setMobileMenuOpen] = useState(false);
  const [hasDashboardPermission, setHasDashboardPermission] = useState(false);
  const [hasWorkspacesPermission, setHasWorkspacesPermission] = useState(false);
  const [hasModulesPermission, setHasModulesPermission] = useState(false);
  const [permissionsLoading, setPermissionsLoading] = useState(true);

  React.useEffect(() => {
    if (!isAuthenticated) {
      navigate('/login');
    }
  }, [isAuthenticated, navigate]);

  // 检查用户是否有DASHBOARD、WORKSPACES和MODULES权限
  React.useEffect(() => {
    const checkPermissions = async () => {
      setPermissionsLoading(true);
      if (user && user.role !== 'admin') {
        try {
          // 使用权限检查API，不暴露具体的业务API
          const checkPermission = async (resourceType: string, scopeType: string = 'ORGANIZATION', scopeId: string = '1') => {
            const response = await fetch('/api/v1/iam/permissions/check', {
              method: 'POST',
              headers: {
                'Content-Type': 'application/json',
                'Authorization': `Bearer ${localStorage.getItem('token')}`
              },
              body: JSON.stringify({
                resource_type: resourceType,
                scope_type: scopeType,
                scope_id: scopeId,
                required_level: 'READ'
              })
            });
            
            if (response.ok) {
              const data = await response.json();
              return data.is_allowed === true;
            }
            return false;
          };

          // 检查各项权限
          const dashPerm = await checkPermission('ORGANIZATION');
          const workspacesPerm = await checkPermission('WORKSPACES');
          const modulesPerm = await checkPermission('MODULES');
          
          console.log('[权限检查] Dashboard:', dashPerm, 'Workspaces:', workspacesPerm, 'Modules:', modulesPerm);
          
          setHasDashboardPermission(dashPerm);
          setHasWorkspacesPermission(workspacesPerm);
          setHasModulesPermission(modulesPerm);
        } catch (error) {
          setHasDashboardPermission(false);
          setHasWorkspacesPermission(false);
          setHasModulesPermission(false);
        }
      } else if (user && user.role === 'admin') {
        setHasDashboardPermission(true);
        setHasWorkspacesPermission(true);
        setHasModulesPermission(true);
      }
      setPermissionsLoading(false);
    };
    
    checkPermissions();
  }, [user]);

  if (!isAuthenticated) {
    return <div>Loading...</div>;
  }

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

  const [expandedMenus, setExpandedMenus] = useState<string[]>([]);

  // Filter navigation menu based on user role
  const allNavItems = [
    { path: '/', label: 'Dashboard', icon: '', requireAdmin: false },
    { path: '/modules', label: 'Modules', icon: '', requireAdmin: false, requireModulesPermission: true },
    { path: '/admin/manifests', label: 'Manifests', icon: '', requireAdmin: true },
    { path: '/workspaces', label: 'Workspaces', icon: '', requireAdmin: false, requireWorkspacesPermission: true },
    { path: '/cmdb', label: 'CMDB', icon: '', requireAdmin: false },
    { path: '/iam/organizations', label: 'IAM', icon: '', requireAdmin: true },
    { 
      path: '/global', 
      label: 'Global Settings', 
      icon: '',
      requireAdmin: true,
      children: [
        { path: '/global/settings/terraform-versions', label: 'IaC Engine', icon: '' },
        { path: '/global/settings/ai-configs', label: 'AI Config', icon: '' },
        { path: '/global/settings/agent-pools', label: 'Agent Pools', icon: '' },
        { path: '/global/settings/run-tasks', label: 'Run Tasks', icon: '' },
        { path: '/global/settings/notifications', label: 'Notifications', icon: '' },
        { path: '/global/settings/platform-config', label: 'Platform Config', icon: '' },
      ]
    },
  ];

  // 根据用户角色和权限过滤菜单
  const navItems = user?.role === 'admin' 
    ? allNavItems 
    : allNavItems.filter(item => {
        // 仪表板：有DASHBOARD权限的用户可以看到
        if (item.path === '/') return hasDashboardPermission;
        // 模块管理：有MODULES权限的用户可以看到
        if (item.path === '/modules') return hasModulesPermission;
        // 工作空间：有WORKSPACES权限的用户可以看到
        if (item.path === '/workspaces') return hasWorkspacesPermission;
        // 其他需要admin的菜单：不显示
        return !item.requireAdmin;
      });

  const toggleMenu = (path: string) => {
    setExpandedMenus(prev => 
      prev.includes(path) 
        ? prev.filter(p => p !== path)
        : [...prev, path]
    );
  };

  const isMenuExpanded = (path: string) => expandedMenus.includes(path);

  const getPageTitle = () => {
    // 先查找一级菜单
    const currentItem = navItems.find(item => item.path === location.pathname);
    if (currentItem) return currentItem.label;
    
    // 查找二级菜单
    for (const item of navItems) {
      if (item.children) {
        const child = item.children.find(c => c.path === location.pathname);
        if (child) return child.label;
      }
    }
    
    return 'IaC Platform';
  };

  const handleMobileNavClick = (path: string) => {
    // 如果是导航到API文档，设置时间戳标记
    if (path === '/api-docs') {
      sessionStorage.setItem('swagger-navigation', Date.now().toString());
    }
    navigate(path);
    setMobileMenuOpen(false);
  };

  // 权限加载中，显示loading
  if (permissionsLoading) {
    return <div>Loading...</div>;
  }

  // 如果用户不是admin且没有任何权限，显示NoPermission页面
  if (user?.role !== 'admin' && !hasDashboardPermission && !hasWorkspacesPermission && !hasModulesPermission) {
    return <NoPermission />;
  }

  return (
    <div className={styles.layout}>
      {/* 移动端汉堡菜单按钮 */}
      <button 
        className={styles.mobileMenuButton}
        onClick={() => setMobileMenuOpen(true)}
        aria-label="Menu"
      >
        ☰
      </button>

      {/* 移动端遮罩层 */}
      {mobileMenuOpen && (
        <div 
          className={styles.mobileOverlay}
          onClick={() => setMobileMenuOpen(false)}
        />
      )}

      {/* 侧边栏 - 桌面端固定显示，移动端作为抽屉 */}
      <aside className={`${styles.sidebar} ${sidebarCollapsed ? styles.collapsed : ''} ${mobileMenuOpen ? styles.mobileOpen : ''}`}>
        <div className={styles.logo}>
          <h1 
            className={styles.logoText}
            onClick={() => navigate('/')}
            style={{ cursor: 'pointer' }}
          >
            {sidebarCollapsed ? 'IaC' : 'IaC Platform'}
          </h1>
          <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
            {/* API按钮 - 仅超管可见，随侧边栏折叠隐藏 */}
            {user?.role === 'admin' && !sidebarCollapsed && (
              <button
                className={styles.apiButton}
                onClick={() => {
                  sessionStorage.setItem('swagger-navigation', Date.now().toString());
                  navigate('/api-docs');
                }}
                title="API Docs"
              >
                <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
                  <polyline points="4 17 10 11 4 5"></polyline>
                  <line x1="12" y1="19" x2="20" y2="19"></line>
                </svg>
              </button>
            )}
            <button 
              className={styles.collapseButton}
              onClick={() => setSidebarCollapsed(!sidebarCollapsed)}
              title={sidebarCollapsed ? 'Expand sidebar' : 'Collapse sidebar'}
            >
              ☰
            </button>
          </div>
        </div>
        
        <nav className={styles.nav}>
          {navItems.map((item) => (
            <div key={item.path}>
              {item.children ? (
                // 有子菜单的项目使用 button 来展开/折叠
                <button
                  className={`${styles.navItem} ${
                    item.children.some(child => location.pathname === child.path) ? styles.active : ''
                  }`}
                  onClick={() => toggleMenu(item.path)}
                  title={sidebarCollapsed ? item.label : ''}
                >
                  <span className={styles.navIcon}>{item.icon}</span>
                  {!sidebarCollapsed && (
                    <>
                      <span className={styles.navLabel}>{item.label}</span>
                      <span className={styles.expandIcon}>
                        {isMenuExpanded(item.path) ? '▼' : '▶'}
                      </span>
                    </>
                  )}
                </button>
              ) : (
                // 没有子菜单的项目使用 Link
                <Link
                  to={item.path}
                  className={`${styles.navItem} ${
                    location.pathname === item.path ? styles.active : ''
                  }`}
                  title={sidebarCollapsed ? item.label : ''}
                  onClick={() => setMobileMenuOpen(false)}
                >
                  <span className={styles.navIcon}>{item.icon}</span>
                  {!sidebarCollapsed && (
                    <span className={styles.navLabel}>{item.label}</span>
                  )}
                </Link>
              )}
              
              {/* 二级菜单 */}
              {item.children && isMenuExpanded(item.path) && !sidebarCollapsed && (
                <div className={styles.subMenu}>
                  {item.children.map((child) => (
                    <Link
                      key={child.path}
                      to={child.path}
                      className={`${styles.subMenuItem} ${
                        location.pathname === child.path ? styles.active : ''
                      }`}
                      onClick={() => setMobileMenuOpen(false)}
                    >
                      <span className={styles.navIcon}>{child.icon}</span>
                      <span className={styles.navLabel}>{child.label}</span>
                    </Link>
                  ))}
                </div>
              )}
            </div>
          ))}
        </nav>
      </aside>
      
      <main className={styles.main}>
        <header className={styles.header}>
          <h2 className={styles.headerTitle}>{getPageTitle()}</h2>
          
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
        
        <div className={styles.content}>
          <div className={styles.contentInner}>
            <Outlet />
          </div>
        </div>
      </main>
    </div>
  );
};

export default Layout;
