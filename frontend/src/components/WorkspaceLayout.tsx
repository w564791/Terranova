/**
 * Workspace context shell — vertical workspace nav + shared TopBar via ContextShell.
 */
import React, { useEffect, useMemo, useState } from 'react';
import { useLocation, useParams } from 'react-router-dom';
import ContextShell from './ContextShell';
import WorkspaceSidebar, {
  type WorkspaceSettingsSection,
  type WorkspaceTabType,
} from './WorkspaceSidebar';
import api from '../services/api';

function deriveActiveTab(pathname: string, search: string): WorkspaceTabType {
  if (pathname.includes('/tasks/')) return 'runs';
  if (pathname.includes('/states/')) return 'states';
  if (pathname.includes('/add-resources')) return 'resources';
  if (pathname.includes('/resources')) return 'resources';

  const tab = new URLSearchParams(search).get('tab') as WorkspaceTabType | null;
  if (
    tab &&
    [
      'overview',
      'runs',
      'states',
      'resources',
      'variables',
      'outputs',
      'health',
      'settings',
    ].includes(tab)
  ) {
    return tab;
  }
  return 'overview';
}

function deriveActiveSection(search: string): WorkspaceSettingsSection {
  const section = new URLSearchParams(search).get('section') as WorkspaceSettingsSection | null;
  if (
    section &&
    [
      'general',
      'locking',
      'provider',
      'run-tasks',
      'run-triggers',
      'notifications',
      'destruction',
    ].includes(section)
  ) {
    return section;
  }
  return 'general';
}

const WorkspaceLayout: React.FC = () => {
  const params = useParams<{ id?: string; workspaceId?: string }>();
  const location = useLocation();
  const workspaceId = params.id || params.workspaceId || '';

  const [workspaceName, setWorkspaceName] = useState('Workspace');

  const activeTab = useMemo(
    () => deriveActiveTab(location.pathname, location.search),
    [location.pathname, location.search]
  );
  const activeSection = useMemo(
    () => deriveActiveSection(location.search),
    [location.search]
  );

  useEffect(() => {
    if (!workspaceId) return;
    let cancelled = false;
    (async () => {
      try {
        const response: any = await api.get(`/workspaces/${workspaceId}`);
        const data = response?.data || response;
        if (!cancelled && data?.name) {
          setWorkspaceName(data.name);
        }
      } catch {
        // keep fallback
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [workspaceId]);

  return (
    <ContextShell
      renderSidebar={(mobile) => (
        <WorkspaceSidebar
          workspaceId={workspaceId}
          workspaceName={workspaceName}
          activeTab={activeTab}
          activeSection={activeSection}
          mobileSidebarOpen={mobile.open}
          onMobileSidebarClose={mobile.closeMenu}
        />
      )}
    />
  );
};

export default WorkspaceLayout;
