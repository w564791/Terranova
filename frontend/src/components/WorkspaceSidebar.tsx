import React, { useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import styles from './WorkspaceSidebar.module.css';

type TabType = 'overview' | 'runs' | 'states' | 'resources' | 'variables' | 'outputs' | 'health' | 'settings';
type SettingsSection = 'general' | 'locking' | 'provider' | 'run-tasks' | 'run-triggers' | 'notifications' | 'destruction';

interface WorkspaceSidebarProps {
  workspaceId: string;
  workspaceName: string;
  activeTab: TabType;
  activeSection?: SettingsSection;
  onTabChange?: (tab: TabType) => void;
  onSectionChange?: (section: SettingsSection) => void;
  mobileSidebarOpen?: boolean;
  onMobileSidebarClose?: () => void;
}

// 导航菜单项
const navItems = [
  { id: 'overview', label: 'Overview' },
  { id: 'runs', label: 'Runs' },
  { id: 'states', label: 'States' },
  { id: 'resources', label: 'Resources' },
  { id: 'variables', label: 'Variables' },
  { id: 'outputs', label: 'Outputs' },
  { id: 'health', label: 'Health' },
];

// Settings子菜单项
const settingsItems = [
  { id: 'general', label: 'General' },
  { id: 'locking', label: 'Locking' },
  { id: 'provider', label: 'Provider' },
  { id: 'run-tasks', label: 'Run Tasks' },
  { id: 'run-triggers', label: 'Run Triggers' },
  { id: 'notifications', label: 'Notifications' },
  { id: 'destruction', label: 'Destruction and Deletion' },
];

const WorkspaceSidebar: React.FC<WorkspaceSidebarProps> = ({
  workspaceId,
  workspaceName,
  activeTab,
  activeSection = 'general',
  onTabChange,
  onSectionChange,
  mobileSidebarOpen = false,
  onMobileSidebarClose,
}) => {
  const navigate = useNavigate();
  const [settingsExpanded, setSettingsExpanded] = useState(activeTab === 'settings');

  const handleTabClick = (tab: TabType) => {
    if (onTabChange) {
      onTabChange(tab);
    } else {
      // 默认导航行为
      if (tab === 'settings') {
        navigate(`/workspaces/${workspaceId}?tab=${tab}&section=${activeSection}`);
      } else {
        navigate(`/workspaces/${workspaceId}?tab=${tab}`);
      }
    }
    onMobileSidebarClose?.();
  };

  const handleSectionClick = (section: SettingsSection) => {
    if (onSectionChange) {
      onSectionChange(section);
    } else {
      navigate(`/workspaces/${workspaceId}?tab=settings&section=${section}`);
    }
    onMobileSidebarClose?.();
  };

  const handleSettingsToggle = () => {
    if (settingsExpanded) {
      setSettingsExpanded(false);
      if (activeTab === 'settings') {
        handleTabClick('overview');
      }
    } else {
      setSettingsExpanded(true);
      handleTabClick('settings');
    }
  };

  return (
    <aside className={`${styles.sidebar} ${mobileSidebarOpen ? styles.sidebarMobileOpen : ''}`}>
      <div className={styles.sidebarHeader}>
        <button onClick={() => navigate('/workspaces')} className={styles.backButton}>
          ← Workspaces
        </button>
        
        <h1 className={styles.workspaceTitle}>{workspaceName}</h1>
      </div>

      {/* 导航菜单 */}
      <nav className={styles.nav}>
        {navItems.map((item) => (
          <Link
            key={item.id}
            to={`/workspaces/${workspaceId}?tab=${item.id}`}
            className={`${styles.navItem} ${
              activeTab === item.id ? styles.navItemActive : ''
            }`}
            onClick={(e) => {
              e.preventDefault();
              handleTabClick(item.id as TabType);
            }}
          >
            <span className={styles.navLabel}>{item.label}</span>
          </Link>
        ))}
        
        {/* Settings可展开菜单 */}
        <button
          className={`${styles.navItem} ${styles.navItemExpandable} ${
            activeTab === 'settings' ? styles.navItemActive : ''
          }`}
          onClick={handleSettingsToggle}
        >
          <span className={styles.navLabel}>Settings</span>
          <span className={`${styles.expandIcon} ${settingsExpanded ? styles.expandIconOpen : ''}`}>
            ▼
          </span>
        </button>
        
        {/* Settings子菜单 */}
        {settingsExpanded && (
          <div className={styles.subMenu}>
            {settingsItems.map((item) => (
              <Link
                key={item.id}
                to={`/workspaces/${workspaceId}?tab=settings&section=${item.id}`}
                className={`${styles.subMenuItem} ${
                  activeTab === 'settings' && activeSection === item.id ? styles.subMenuItemActive : ''
                }`}
                onClick={(e) => {
                  e.preventDefault();
                  handleSectionClick(item.id as SettingsSection);
                }}
              >
                <span className={styles.navLabel}>{item.label}</span>
              </Link>
            ))}
          </div>
        )}
      </nav>
    </aside>
  );
};

export default WorkspaceSidebar;
