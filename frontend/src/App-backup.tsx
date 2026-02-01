import type { FC } from 'react';
import { BrowserRouter as Router, Routes, Route } from 'react-router-dom';
import { ConfigProvider } from 'antd';
import zhCN from 'antd/es/locale/zh_CN';
import { Provider } from 'react-redux';
import { store } from './store';
import Layout from './components/Layout';
import Login from './pages/Login';
import Dashboard from './pages/Dashboard';
import ResetPassword from './pages/ResetPassword';
import Modules from './pages/Modules';
import ModuleDetail from './pages/ModuleDetail';
import CreateModule from './pages/CreateModule';
import EditModule from './pages/EditModule';
import Workspaces from './pages/Workspaces';
import WorkspaceDetail from './pages/WorkspaceDetail';
import CreateWorkspace from './pages/CreateWorkspace';
import EditWorkspace from './pages/EditWorkspace';
import './App.css';

console.log('App component loaded');

const App: FC = () => {
  console.log('App rendering');
  
  try {
    return (
      <Provider store={store}>
        <ConfigProvider locale={zhCN}>
          <Router>
            <Routes>
              <Route path="/login" element={<Login />} />
              <Route path="/reset-password" element={<ResetPassword />} />
              <Route path="/" element={<Layout />}>
                <Route index element={<Dashboard />} />
                <Route path="modules" element={<Modules />} />
                <Route path="modules/create" element={<CreateModule />} />
                <Route path="modules/:id" element={<ModuleDetail />} />
                <Route path="modules/:id/edit" element={<EditModule />} />
                <Route path="workspaces" element={<Workspaces />} />
                <Route path="workspaces/create" element={<CreateWorkspace />} />
                <Route path="workspaces/:id" element={<WorkspaceDetail />} />
                <Route path="workspaces/:id/edit" element={<EditWorkspace />} />
              </Route>
            </Routes>
          </Router>
        </ConfigProvider>
      </Provider>
    );
  } catch (error) {
    console.error('App render error:', error);
    return <div>Loading error: {String(error)}</div>;
  }
};

export default App;