import type { FC } from 'react';
import { BrowserRouter as Router, Routes, Route } from 'react-router-dom';
import { Provider } from 'react-redux';
import { store } from './store';
import { ToastProvider } from './contexts/ToastContext';
import AuthProvider from './components/AuthProvider';
import Layout from './components/Layout';
import ProtectedRoute from './components/ProtectedRoute';
import Login from './pages/Login';
import Dashboard from './pages/Dashboard';
import ResetPassword from './pages/ResetPassword';
import Modules from './pages/Modules';
import ModuleDetail from './pages/ModuleDetail';
import CreateModule from './pages/CreateModule';
import ImportModule from './pages/ImportModule';
import EditModule from './pages/EditModule';
import DemoDetail from './pages/DemoDetail';
import EditDemo from './pages/EditDemo';
import CreateDemo from './pages/CreateDemo';
import ModuleDemos from './pages/ModuleDemos';
import ModuleVersionSkill from './pages/ModuleVersionSkill';
import SkillDetail from './pages/SkillDetail';
import SkillCreate from './pages/SkillCreate';
import Workspaces from './pages/Workspaces';
import WorkspaceDetail from './pages/WorkspaceDetail';
import CreateWorkspace from './pages/CreateWorkspace';
import WorkspaceSettings from './pages/WorkspaceSettings';
import EditWorkspace from './pages/EditWorkspace';
import WorkspaceResources from './pages/WorkspaceResources';
import AddResources from './pages/AddResources';
import EditResource from './pages/EditResource';
import ViewResource from './pages/ViewResource';
import TaskDetail from './pages/TaskDetail';
import StatePreview from './pages/StatePreview';
import TestDynamicForm from './pages/TestDynamicForm';
import SchemaManagement from './pages/SchemaManagement';
import SchemaEditorPage from './pages/SchemaEditorPage';
import Admin from './pages/Admin';
import AIConfigList from './pages/AIConfigList';
import AIConfigForm from './pages/AIConfigForm';
import OrganizationManagement from './pages/admin/OrganizationManagement';
import ProjectManagement from './pages/admin/ProjectManagement';
import TeamManagement from './pages/admin/TeamManagement';
import TeamDetail from './pages/admin/TeamDetail';
import PermissionManagement from './pages/admin/PermissionManagement';
import GrantPermission from './pages/admin/GrantPermission';
import RoleManagement from './pages/admin/RoleManagement';
import UserManagement from './pages/admin/UserManagement';
import ApplicationManagement from './pages/admin/ApplicationManagement';
import AuditLog from './pages/admin/AuditLog';
import AgentPoolManagement from './pages/admin/AgentPoolManagement';
import AgentPoolDetail from './pages/admin/AgentPoolDetail';
import AgentPoolForm from './pages/admin/AgentPoolForm';
import RunTaskManagement from './pages/admin/RunTaskManagement';
import RunTaskForm from './pages/admin/RunTaskForm';
import NotificationManagement from './pages/admin/NotificationManagement';
import NotificationForm from './pages/admin/NotificationForm';
import PlatformConfig from './pages/admin/PlatformConfig';
import ManifestManagement from './pages/admin/ManifestManagement';
import ManifestCreate from './pages/admin/ManifestCreate';
import ManifestEditor from './pages/admin/ManifestEditor';
import ManifestDeploy from './pages/admin/ManifestDeploy';
import IAMLayout from './components/IAMLayout';
import SwaggerUI from './pages/SwaggerUI';
import PersonalSettings from './pages/PersonalSettings';
import CMDB from './pages/CMDB';
import Setup from './pages/Setup';
import SSOCallback from './pages/SSOCallback';
import SSOConfig from './pages/admin/SSOConfig';
import MFASetup from './pages/MFASetup';
import MFAVerify from './pages/MFAVerify';
import MFAConfig from './pages/admin/MFAConfig';
import './App.css';

console.log('App component loaded');

const App: FC = () => {
  console.log('App rendering');
  
  try {
    return (
      <Provider store={store}>
        <ToastProvider>
          <AuthProvider>
            <Router>
            <Routes>
              <Route path="/setup" element={<Setup />} />
              <Route path="/login" element={<Login />} />
              <Route path="/sso/callback" element={<SSOCallback />} />
              <Route path="/login/mfa" element={<MFAVerify />} />
              <Route path="/setup/mfa" element={<MFASetup />} />
              <Route path="/reset-password" element={<ResetPassword />} />
              
              {/* WorkspaceDetail 独立路由，不使用 Layout */}
              <Route path="/workspaces/:id" element={
                <ProtectedRoute>
                  <WorkspaceDetail />
                </ProtectedRoute>
              } />
              
              {/* TaskDetail 独立路由 */}
              <Route path="/workspaces/:workspaceId/tasks/:taskId" element={
                <ProtectedRoute>
                  <TaskDetail />
                </ProtectedRoute>
              } />
              
              {/* StatePreview 独立路由 */}
              <Route path="/workspaces/:workspaceId/states/:version" element={
                <ProtectedRoute>
                  <StatePreview />
                </ProtectedRoute>
              } />
              
              {/* EditResource 独立路由 - 使用 Workspace 侧边栏 */}
              <Route path="/workspaces/:id/resources/:resourceId/edit" element={
                <ProtectedRoute>
                  <EditResource />
                </ProtectedRoute>
              } />
              
              {/* AddResources 独立路由 - 使用 Workspace 侧边栏 */}
              <Route path="/workspaces/:id/add-resources" element={
                <ProtectedRoute>
                  <AddResources />
                </ProtectedRoute>
              } />
              
              {/* ViewResource 独立路由 - 使用 Workspace 侧边栏 */}
              <Route path="/workspaces/:id/resources/:resourceId" element={
                <ProtectedRoute>
                  <ViewResource />
                </ProtectedRoute>
              } />
              
              {/* 其他页面使用 Layout */}
              <Route path="/" element={
                <ProtectedRoute>
                  <Layout />
                </ProtectedRoute>
              }>
                <Route index element={<Dashboard />} />
                <Route path="modules" element={<Modules />} />
                <Route path="modules/create" element={<CreateModule />} />
                <Route path="modules/import" element={<ImportModule />} />
                <Route path="modules/:id" element={<ModuleDetail />} />
                <Route path="modules/:id/edit" element={<EditModule />} />
                <Route path="modules/:moduleId/demos" element={<ModuleDemos />} />
                <Route path="modules/:moduleId/demos/create" element={<CreateDemo />} />
                <Route path="modules/:moduleId/demos/:demoId" element={<DemoDetail />} />
                <Route path="modules/:moduleId/demos/:demoId/edit" element={<EditDemo />} />
                <Route path="modules/:moduleId/skill" element={<ModuleVersionSkill />} />
                <Route path="workspaces" element={<Workspaces />} />
                <Route path="workspaces/create" element={<CreateWorkspace />} />
                <Route path="workspaces/:id/edit" element={<EditWorkspace />} />
                <Route path="workspaces/:id/resources" element={<WorkspaceResources />} />
                <Route path="test-form" element={<TestDynamicForm />} />
                <Route path="modules/:moduleId/schemas" element={<SchemaManagement />} />
                <Route path="modules/:moduleId/schemas/:schemaId/edit" element={<SchemaEditorPage />} />
                <Route path="global/settings/terraform-versions" element={<Admin />} />
                <Route path="global/settings/ai-configs" element={<AIConfigList />} />
                <Route path="global/settings/ai-configs/create" element={<AIConfigForm />} />
                <Route path="global/settings/ai-configs/:id/edit" element={<AIConfigForm />} />
                <Route path="global/settings/skills/create" element={<SkillCreate />} />
                <Route path="global/settings/skills/:id" element={<SkillDetail />} />
                <Route path="global/settings/agent-pools" element={<AgentPoolManagement />} />
                <Route path="global/settings/agent-pools/create" element={<AgentPoolForm />} />
                <Route path="global/settings/agent-pools/:poolId" element={<AgentPoolDetail />} />
                <Route path="global/settings/agent-pools/:poolId/edit" element={<AgentPoolForm />} />
                <Route path="global/settings/run-tasks" element={<RunTaskManagement />} />
                <Route path="global/settings/run-tasks/create" element={<RunTaskForm />} />
                <Route path="global/settings/run-tasks/:runTaskId/edit" element={<RunTaskForm />} />
                <Route path="global/settings/notifications" element={<NotificationManagement />} />
                <Route path="global/settings/notifications/create" element={<NotificationForm />} />
                <Route path="global/settings/notifications/:notificationId/edit" element={<NotificationForm />} />
                <Route path="global/settings/platform-config" element={<PlatformConfig />} />
                <Route path="global/settings/mfa" element={<MFAConfig />} />
                <Route path="global/settings/sso" element={<SSOConfig />} />
                <Route path="admin/manifests" element={<ManifestManagement />} />
                <Route path="admin/manifests/new" element={<ManifestCreate />} />
                <Route path="api-docs" element={<SwaggerUI />} />
                <Route path="settings" element={<PersonalSettings />} />
                <Route path="settings/mfa" element={<MFASetup />} />
                <Route path="cmdb" element={<CMDB />} />
              </Route>
              
              {/* Manifest 编辑器 - 独立全屏路由 */}
              <Route path="/admin/manifests/:id/edit" element={
                <ProtectedRoute>
                  <ManifestEditor />
                </ProtectedRoute>
              } />
              
              {/* Manifest 部署页面 */}
              <Route path="/admin/manifests/:id/deploy" element={
                <ProtectedRoute>
                  <ManifestDeploy />
                </ProtectedRoute>
              } />
              
              {/* Manifest 部署页面（复数形式，兼容） */}
              <Route path="/admin/manifests/:id/deployments" element={
                <ProtectedRoute>
                  <ManifestDeploy />
                </ProtectedRoute>
              } />
              
              {/* IAM子系统 - 使用独立的IAMLayout */}
              <Route path="/iam" element={
                <ProtectedRoute>
                  <IAMLayout />
                </ProtectedRoute>
              }>
                <Route path="organizations" element={<OrganizationManagement />} />
                <Route path="projects" element={<ProjectManagement />} />
                <Route path="users" element={<UserManagement />} />
                <Route path="teams" element={<TeamManagement />} />
                <Route path="teams/:id" element={<TeamDetail />} />
                <Route path="applications" element={<ApplicationManagement />} />
                <Route path="permissions" element={<PermissionManagement />} />
                <Route path="permissions/grant" element={<GrantPermission />} />
                <Route path="roles" element={<RoleManagement />} />
                <Route path="audit" element={<AuditLog />} />
              </Route>
            </Routes>
            </Router>
          </AuthProvider>
        </ToastProvider>
      </Provider>
    );
  } catch (error) {
    console.error('App render error:', error);
    return <div>Loading error: {String(error)}</div>;
  }
};

export default App;
