/**
 * Shared context chrome for Workspace / IAM (and future sub-apps).
 * Owns: fixed left rail slot + TopBar + content outlet. Sidebar content is injected.
 * Do not reimplement TopBar or shell layout in feature pages.
 */
import React, { useEffect, useState } from 'react';
import { Outlet, useLocation } from 'react-router-dom';
import TopBar from './TopBar';
import styles from './ContextShell.module.css';

export interface ContextShellMobile {
  open: boolean;
  openMenu: () => void;
  closeMenu: () => void;
}

export interface ContextShellProps {
  /** Left rail (vertical nav). Receives mobile open state for drawer behavior. */
  renderSidebar: (mobile: ContextShellMobile) => React.ReactNode;
}

const ContextShell: React.FC<ContextShellProps> = ({ renderSidebar }) => {
  const location = useLocation();
  const [mobileOpen, setMobileOpen] = useState(false);

  useEffect(() => {
    setMobileOpen(false);
  }, [location.pathname, location.search]);

  const mobile: ContextShellMobile = {
    open: mobileOpen,
    openMenu: () => setMobileOpen(true),
    closeMenu: () => setMobileOpen(false),
  };

  return (
    <div className={styles.shell}>
      <button
        type="button"
        className={styles.mobileMenuButton}
        onClick={mobile.openMenu}
        aria-label="Open menu"
      >
        ☰
      </button>

      {mobileOpen && (
        <div className={styles.mobileOverlay} onClick={mobile.closeMenu} />
      )}

      {renderSidebar(mobile)}

      <div className={styles.main}>
        <TopBar />
        <div className={styles.content}>
          <Outlet />
        </div>
      </div>
    </div>
  );
};

export default ContextShell;
