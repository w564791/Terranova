/**
 * IAM context shell — same chrome as Workspace (ContextShell + TopBar),
 * different menu content (IAMSidebar).
 */
import React from 'react';
import ContextShell from './ContextShell';
import IAMSidebar from './IAMSidebar';

const IAMLayout: React.FC = () => {
  return (
    <ContextShell
      renderSidebar={(mobile) => (
        <IAMSidebar
          mobileSidebarOpen={mobile.open}
          onMobileSidebarClose={mobile.closeMenu}
        />
      )}
    />
  );
};

export default IAMLayout;
