# 路由重构方案

##  重要说明
**本次重构仅涉及前端路由调整，后端API路径保持不变。**

-  **前端优化**：调整前端路由路径和导航菜单
- ❌ **后端无需修改**：后端API已经使用 `/api/v1/iam/...` 路径，符合规范

## 当前问题
1. 系统管理（Admin）路径混乱，包含了设置页面和IAM管理页面
2. IAM管理系统的路径缺少 `/iam/` 前缀
3. 导航菜单命名不够清晰

## 重构目标

### 1. 路径结构调整

#### 全局设置（Global Settings）
将系统设置相关页面从 `/admin/` 移到 `/global/settings/`：

**当前路径** → **新路径**
- `/admin/terraform-versions` → `/global/settings/terraform-versions`
- `/admin/ai-configs` → `/global/settings/ai-configs`
- `/admin/ai-configs/create` → `/global/settings/ai-configs/create`
- `/admin/ai-configs/:id/edit` → `/global/settings/ai-configs/:id/edit`

#### IAM管理系统
为所有IAM管理页面添加 `/iam/` 前缀：

**当前路径** → **新路径**
- `/admin/organizations` → `/iam/organizations`
- `/admin/projects` → `/iam/projects`
- `/admin/users` → `/iam/users`
- `/admin/teams` → `/iam/teams`
- `/admin/applications` → `/iam/applications`
- `/admin/permissions` → `/iam/permissions`
- `/admin/permissions/grant` → `/iam/permissions/grant`
- `/admin/roles` → `/iam/roles`
- `/admin/audit` → `/iam/audit`

### 2. 导航菜单调整

#### Layout.tsx 主导航
```typescript
const allNavItems = [
  { path: '/', label: '仪表板', icon: '', requireAdmin: false },
  { path: '/modules', label: '模块管理', icon: '', requireAdmin: false },
  { path: '/workspaces', label: '工作空间', icon: '', requireAdmin: false },
  { 
    path: '/global', 
    label: '全局设置',  // 从"系统管理"改为"全局设置"
    icon: '',
    requireAdmin: true,
    children: [
      { path: '/global/settings/terraform-versions', label: 'Terraform版本', icon: '' },
      { path: '/global/settings/ai-configs', label: 'AI配置', icon: '' },
      { path: '/iam/organizations', label: 'IAM管理', icon: '' },  // 入口指向IAM
    ]
  },
];
```

#### IAMLayout.tsx IAM导航
```typescript
const navItems = [
  { path: '/iam/organizations', label: '组织管理', icon: '' },
  { path: '/iam/projects', label: '项目管理', icon: '' },
  { path: '/iam/users', label: '用户管理', icon: '' },
  { path: '/iam/teams', label: '团队管理', icon: '' },
  { path: '/iam/applications', label: '应用管理', icon: '' },
  { path: '/iam/permissions', label: '权限管理', icon: '' },
  { path: '/iam/roles', label: '角色管理', icon: '' },
  { path: '/iam/audit', label: '审计日志', icon: '' },
];
```

### 3. 需要修改的文件

#### 🎨 前端文件（需要修改）
1. **frontend/src/App.tsx** - 路由配置
2. **frontend/src/components/Layout.tsx** - 主导航菜单
3. **frontend/src/components/IAMLayout.tsx** - IAM导航菜单
4. 可能需要修改的页面组件中的导航链接

####  后端文件（无需修改）
后端API路径已经规范化为 `/api/v1/iam/...`，完全符合要求，无需任何修改。

**前后端路径对应关系：**
- 前端路由：`/iam/users` → 调用后端API：`/api/v1/iam/users` 
- 前端路由：`/iam/organizations` → 调用后端API：`/api/v1/iam/organizations` 
- 前端路由：`/global/settings/terraform-versions` → 调用后端API：`/api/v1/admin/terraform-versions` 

### 4. 实施步骤

#### 步骤1: 更新 App.tsx 路由配置
```typescript
// 全局设置路由（使用 Layout）
<Route path="/global/settings/terraform-versions" element={<Admin />} />
<Route path="/global/settings/ai-configs" element={<AIConfigList />} />
<Route path="/global/settings/ai-configs/create" element={<AIConfigForm />} />
<Route path="/global/settings/ai-configs/:id/edit" element={<AIConfigForm />} />

// IAM管理路由（使用 IAMLayout）
<Route path="/iam" element={<ProtectedRoute><IAMLayout /></ProtectedRoute>}>
  <Route path="organizations" element={<OrganizationManagement />} />
  <Route path="projects" element={<ProjectManagement />} />
  <Route path="users" element={<UserManagement />} />
  <Route path="teams" element={<TeamManagement />} />
  <Route path="applications" element={<ApplicationManagement />} />
  <Route path="permissions" element={<PermissionManagement />} />
  <Route path="permissions/grant" element={<GrantPermission />} />
  <Route path="roles" element={<RoleManagement />} />
  <Route path="audit" element={<AuditLog />} />
</Route>
```

#### 步骤2: 更新 Layout.tsx 导航菜单
- 将"系统管理"改为"全局设置"
- 更新子菜单路径
- 更新IAM入口链接

#### 步骤3: 更新 IAMLayout.tsx 导航菜单
- 更新所有导航项的路径，添加 `/iam/` 前缀

#### 步骤4: 添加路由重定向（可选）
为了向后兼容，可以添加重定向：
```typescript
// 旧路径重定向到新路径
<Route path="/admin/terraform-versions" element={<Navigate to="/global/settings/terraform-versions" replace />} />
<Route path="/admin/ai-configs" element={<Navigate to="/global/settings/ai-configs" replace />} />
<Route path="/admin/organizations" element={<Navigate to="/iam/organizations" replace />} />
// ... 其他重定向
```

### 5. 测试清单

- [ ] 全局设置页面可以正常访问
  - [ ] Terraform版本管理
  - [ ] AI配置列表
  - [ ] AI配置创建
  - [ ] AI配置编辑
- [ ] IAM管理页面可以正常访问
  - [ ] 组织管理
  - [ ] 项目管理
  - [ ] 用户管理（包括新增/删除功能）
  - [ ] 团队管理
  - [ ] 应用管理
  - [ ] 权限管理
  - [ ] 角色管理
  - [ ] 审计日志
- [ ] 导航菜单正确显示
  - [ ] 主导航显示"全局设置"
  - [ ] IAM导航显示所有IAM菜单项
- [ ] 面包屑导航正确
- [ ] 返回主系统功能正常

### 6. 风险评估

**低风险**
- 前端路由修改，不影响后端API
- 可以添加重定向保持向后兼容
- 修改范围明确，易于回滚

**注意事项**
- 需要清理浏览器缓存测试
- 检查是否有硬编码的路径引用
- 更新相关文档

## 确认事项

请确认以下内容后再执行：

1.  路径结构是否符合预期？
   - 全局设置：`/global/settings/...`
   - IAM管理：`/iam/...`

2.  导航菜单命名是否合适？
   - "全局设置"（替代"系统管理"）
   - "IAM管理"（作为入口）

3.  是否需要添加旧路径的重定向？

4.  是否有其他需要调整的地方？

确认无误后，我将开始执行路由重构。
