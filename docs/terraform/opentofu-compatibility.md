# OpenTofu 兼容性实现

## 概述

IAC Platform 现已支持 Terraform 和 OpenTofu 两种 IaC 引擎。系统会根据下载链接**运行时自动识别**引擎类型，无需用户手动指定，**无需数据库变更**。

## 实现方案

### 核心设计：运行时推断

引擎类型不存储在数据库中，而是根据下载链接动态推断：

```go
func DetectEngineTypeFromURL(downloadURL string) IaCEngineType {
    url := strings.ToLower(downloadURL)
    
    // OpenTofu 特征检测
    if strings.Contains(url, "opentofu") ||
        strings.Contains(url, "tofu_") ||
        strings.Contains(url, "/tofu/") {
        return IaCEngineOpenTofu
    }
    
    // 默认为 Terraform
    return IaCEngineTerraform
}
```

### 支持的引擎类型

- `terraform` - HashiCorp Terraform
- `opentofu` - OpenTofu (Terraform 的开源分支)

### 二进制文件处理

下载器会根据引擎类型查找正确的二进制文件：
- Terraform: 查找 `terraform` 二进制
- OpenTofu: 查找 `tofu` 二进制

为保持向后兼容，安装后的二进制文件统一命名为 `terraform`，这样现有的执行器无需修改。

## 使用方法

### 添加 Terraform 版本

使用 HashiCorp 官方下载链接：
```
https://releases.hashicorp.com/terraform/1.5.0/terraform_1.5.0_linux_amd64.zip
```

系统会自动识别为 Terraform 引擎。

### 添加 OpenTofu 版本

使用 OpenTofu 官方下载链接：
```
https://github.com/opentofu/opentofu/releases/download/v1.6.0/tofu_1.6.0_linux_amd64.zip
```

系统会自动识别为 OpenTofu 引擎。

## 前端界面

管理页面已更名为 "IaC Engine Versions"，显示：
- 引擎类型徽章（Terraform 紫色，OpenTofu 黄色）
- 版本号
- 下载链接
- 状态

## 兼容性说明

1. **无数据库变更**：引擎类型完全通过运行时推断，不需要修改数据库 schema
2. **向后兼容**：现有的 Terraform 版本配置无需修改
3. **API 兼容**：所有 API 端点保持不变
4. **执行兼容**：Terraform 和 OpenTofu 使用相同的命令行接口，执行流程无需修改

## 执行模式支持

所有三种执行模式都已支持 OpenTofu：

| 执行模式 | 支持状态 | 说明 |
|---------|---------|------|
| **Local** | ✅ 已支持 | 使用 `TerraformDownloader` 直接从数据库获取版本配置 |
| **Agent** | ✅ 已支持 | 通过 `AgentAPIClient.GetTerraformVersion()` 从服务器获取版本配置 |
| **K8s Agent** | ✅ 已支持 | 与 Agent 模式相同，使用 `RemoteDataAccessor` |

### 工作原理

1. **版本配置获取**：
   - Local 模式：从数据库直接查询 `terraform_versions` 表
   - Agent/K8s 模式：通过 API `/api/v1/agents/terraform-versions/{version}` 获取

2. **引擎类型推断**：
   - 所有模式都使用 `GetEngineType()` 方法从 `download_url` 动态推断
   - 无需在数据库中存储引擎类型

3. **二进制下载**：
   - `TerraformDownloader.downloadAndInstall()` 根据引擎类型查找正确的二进制文件
   - OpenTofu 包中的 `tofu` 二进制会被重命名为 `terraform` 以保持兼容

## 技术细节

### 修改的文件

1. `backend/internal/models/terraform_version.go` - 添加引擎类型定义和检测函数
2. `backend/services/terraform_downloader.go` - 支持下载和安装 OpenTofu
3. `frontend/src/services/admin.ts` - 添加引擎类型接口和检测函数
4. `frontend/src/pages/Admin.tsx` - 显示引擎类型
5. `frontend/src/pages/Admin.module.css` - 引擎类型徽章样式

### OpenTofu 下载链接格式

OpenTofu 官方发布地址：
- GitHub Releases: `https://github.com/opentofu/opentofu/releases`
- 文件命名格式: `tofu_<version>_<os>_<arch>.zip`

示例：
- Linux AMD64: `tofu_1.6.0_linux_amd64.zip`
- macOS ARM64: `tofu_1.6.0_darwin_arm64.zip`
- Windows AMD64: `tofu_1.6.0_windows_amd64.zip`

## 检测规则

系统通过以下规则检测引擎类型：

| 下载链接特征 | 引擎类型 |
|-------------|---------|
| 包含 `opentofu` | OpenTofu |
| 包含 `tofu_` | OpenTofu |
| 包含 `/tofu/` | OpenTofu |
| 其他 | Terraform |
