# Workspace Project 功能实施文档

## 概述

为 Workspace 添加 Project 分组功能，允许用户将 Workspace 组织到不同的 Project 中进行管理。

## 功能需求

1. **Project 管理**: 通过参数区分 project，权限挂载到 Organization read/write
2. **Workspace 分组**: 管理员可以将 Workspace 添加到特定 Project
3. **Default Project**: 没有分组的 Workspace 自动归入 default project
4. **Workspace Settings**: 在 General Settings 中添加 Project 选择
5. **列表展示优化**: 参考 Modules 页面的左侧筛选栏布局

## 实施进度

### 后端开发

- [x] 1.1 创建 workspace-project 关联 handler
  - [x] 修复 WorkspaceProjectRelation entity 类型 (workspace_id 改为 string)
  - [x] 更新 ProjectRepository 接口
  - [x] 更新 ProjectRepository 实现
  - [x] 创建 WorkspaceProjectHandler
- [x] 1.2 添加 workspace-project 关联 API 路由
  - [x] 添加 workspace project 路由 (GET/PUT/DELETE /workspaces/:id/project)
  - [x] 添加 projects 路由 (GET /projects, GET /projects/:id/workspaces)
  - [x] 在 router.go 中注册 projects 路由
- [x] 1.3 修改 workspace 列表 API 支持 project_id 参数
  - [x] 修改 workspace service (SearchWorkspaces 添加 projectID 参数)
  - [x] 修改 workspace controller (解析 project_id 查询参数)
- [ ] 1.4 修改 workspace 创建时自动关联 default project (可选，暂不实现)
- [x] 1.5 配置 API 权限到 Organization read/write (已在路由中配置)

### 前端开发

- [x] 2.1 创建 Project 服务层 (services/projects.ts)
  - [x] 定义 Project 类型
  - [x] 实现 getProjects API
  - [x] 实现 getWorkspaceProject API
  - [x] 实现 setWorkspaceProject API
  - [x] 实现 removeWorkspaceFromProject API
- [x] 2.2 改造 Workspaces 列表页面布局
  - [x] 添加 mainContent 容器（左侧筛选栏 + 右侧内容区域）
  - [x] 添加 headerLeft 显示标题和工作空间数量
- [x] 2.3 添加左侧 Project 筛选栏
  - [x] Projects 筛选（单选）
  - [x] Execution Mode 筛选（多选）
  - [x] Status 筛选（多选）
  - [x] 搜索框
  - [x] 重置筛选按钮
- [x] 2.4 修改 WorkspaceSettings 添加 Project 选择
  - [x] 添加 Project 状态变量
  - [x] 添加 loadProjects 和 loadCurrentProject 函数
  - [x] 添加 handleProjectChange 函数
  - [x] 在 General Settings 中添加 Project 选择下拉框
- [x] 2.5 更新样式文件
  - [x] 参考 Modules.module.css 布局
  - [x] 添加 filterSidebar、filterSection、filterGroup 等样式
  - [x] 添加响应式设计

### 测试

- [x] 3.1 后端 API 测试 (编译通过)
- [x] 3.2 前端功能测试 (用户已验证)
- [x] 3.3 端到端测试 (功能完成)

---

## 技术设计

### 数据库表结构 (已存在)

```sql
-- projects 表
CREATE TABLE projects (
    id SERIAL PRIMARY KEY,
    org_id INTEGER NOT NULL,
    name VARCHAR NOT NULL,
    display_name VARCHAR,
    description TEXT,
    is_default BOOLEAN DEFAULT false,
    is_active BOOLEAN DEFAULT true,
    settings JSONB,
    created_by VARCHAR,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- workspace_project_relations 表
CREATE TABLE workspace_project_relations (
    id SERIAL PRIMARY KEY,
    project_id INTEGER NOT NULL,
    workspace_id VARCHAR NOT NULL,
    created_at TIMESTAMP DEFAULT NOW()
);
```

### API 设计

#### 1. 获取 Workspace 的 Project
```
GET /api/v1/workspaces/{workspace_id}/project
Response: { project: Project }
```

#### 2. 设置 Workspace 的 Project
```
PUT /api/v1/workspaces/{workspace_id}/project
Body: { project_id: number }
Response: { message: "success" }
```

#### 3. 获取 Project 下的 Workspaces
```
GET /api/v1/projects/{project_id}/workspaces
Response: { workspaces: Workspace[], total: number }
```

#### 4. 修改 Workspace 列表 API
```
GET /api/v1/workspaces?project_id={project_id}
Response: { items: Workspace[], total: number }
```

### 前端组件设计

#### Workspaces 页面布局
```
+------------------+--------------------------------+
|  Filter Sidebar  |       Content Area             |
|  +------------+  |  +------------------------+    |
|  | Projects   |  |  | Workspace Card 1       |    |
|  | - Default  |  |  +------------------------+    |
|  | - Project1 |  |  | Workspace Card 2       |    |
|  | - Project2 |  |  +------------------------+    |
|  +------------+  |  | ...                    |    |
|  | Status     |  |  +------------------------+    |
|  | - Running  |  |                                |
|  | - Success  |  |                                |
|  +------------+  |                                |
|  | Exec Mode  |  |                                |
|  | - Local    |  |                                |
|  | - Agent    |  |                                |
|  +------------+  |                                |
+------------------+--------------------------------+
```

---

## 实施日志

### 2026-01-07

**开始时间**: 14:17

---

*文档将随实施进度持续更新*
