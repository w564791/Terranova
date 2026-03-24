## v0.4.9

CMDB 搜索结果 inline 展开详情、新增 mock endpoint 资源。

### Features

- **搜索结果 inline 展开详情** — external 资源点击后原地向下展开详情面板（accordion），显示 attributes、tags、resource_summary；点击空白处收起；展开有 slideDown 动画；terraform 资源仍通过 jump_url 跳转 (`CMDB.tsx`, `CMDB.module.css`)
- **CMDB 测试数据** — cmdb-test-server 新增 `sg-1234566`（API Endpoint 安全组）和 `vpce-0a1b2c3d4e5f67890`（VPC Endpoint，绑定 sg-1234566 + subnet-01bc9ccfe9259b6e7）(`cmdb-test-server/main.go`)

### Full Changelog

https://github.com/w564791/iac-platform/compare/v0.4.8...v0.4.9
