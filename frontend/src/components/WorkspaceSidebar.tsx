/**
 * Workspace 上下文左侧导航（竖向）。
 * 进入 workspace 后替代全局 Layout 侧栏；与 TopBar 一并只在 WorkspaceLayout 中挂载一次。
 */
import React, { useEffect, useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import styles from './WorkspaceSidebar.module.css';

export type WorkspaceTabType =
  | 'overview'
  | 'runs'
  | 'states'
  | 'resources'
  | 'variables'
  | 'outputs'
  | 'health'
  | 'settings';

export type WorkspaceSettingsSection =
  | 'general'
  | 'locking'
  | 'provider'
  | 'run-tasks'
  | 'run-triggers'
  | 'notifications'
  | 'destruction';

export interface WorkspaceSidebarProps {
  workspaceId: string;
  workspaceName: string;
  activeTab: WorkspaceTabType;
  activeSection?: WorkspaceSettingsSection;
  onTabChange?: (tab: WorkspaceTabType) => void;
  onSectionChange?: (section: WorkspaceSettingsSection) => void;
  mobileSidebarOpen?: boolean;
  onMobileSidebarClose?: () => void;
}

const navItems: { id: WorkspaceTabType; label: string }[] = [
  { id: 'overview', label: 'Overview' },
  { id: 'runs', label: 'Runs' },
  { id: 'states', label: 'States' },
  { id: 'resources', label: 'Resources' },
  { id: 'variables', label: 'Variables' },
  { id: 'outputs', label: 'Outputs' },
  { id: 'health', label: 'Health' },
];

const settingsItems: { id: WorkspaceSettingsSection; label: string }[] = [
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

  useEffect(() => {
    if (activeTab === 'settings') {
      setSettingsExpanded(true);
    }
  }, [activeTab]);

  const handleTabClick = (tab: WorkspaceTabType) => {
    if (onTabChange) {
      onTabChange(tab);
    } else if (tab === 'settings') {
      navigate(`/workspaces/${workspaceId}?tab=${tab}&section=${activeSection}`);
    } else {
      navigate(`/workspaces/${workspaceId}?tab=${tab}`);
    }
    onMobileSidebarClose?.();
  };

  const handleSectionClick = (section: WorkspaceSettingsSection) => {
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
        <button
          type="button"
          onClick={() => navigate('/workspaces')}
          className={styles.backButton}
        >
          ← Workspaces
        </button>
        <h1 className={styles.workspaceTitle}>{workspaceName}</h1>
      </div>

      <nav className={styles.nav} aria-label="Workspace">
        {navItems.map((item) => (
          <Link
            key={item.id}
            to={`/workspaces/${workspaceId}?tab=${item.id}`}
            className={`${styles.navItem} ${activeTab === item.id ? styles.navItemActive : ''}`}
            onClick={(e) => {
              e.preventDefault();
              handleTabClick(item.id);
            }}
          >
            <span className={styles.navLabel}>{item.label}</span>
          </Link>
        ))}

        <button
          type="button"
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

        {settingsExpanded && (
          <div className={styles.subMenu}>
            {settingsItems.map((item) => (
              <Link
                key={item.id}
                to={`/workspaces/${workspaceId}?tab=settings&section=${item.id}`}
                className={`${styles.subMenuItem} ${
                  activeTab === 'settings' && activeSection === item.id
                    ? styles.subMenuItemActive
                    : ''
                }`}
                onClick={(e) => {
                  e.preventDefault();
                  handleSectionClick(item.id);
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
