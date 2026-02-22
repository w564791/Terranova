## v0.2.9

安全加固专项：修复多项 Critical/High 级别安全漏洞，统一日志规范，强化认证与权限体系。

### Security Fixes

- **JWT 算法混淆防护** — `JWTAuth` 中间件显式验证 HMAC 签名类型 + `jwt.WithValidMethods([]string{"HS256"})`，阻止 `alg:none` 等算法混淆攻击 (`middleware.go`)
- **JWT_SECRET 硬编码消除** — `GetJWTSecret()` 未设置时直接 panic 阻止启动；移除 `team_token_service.go`、`user_token_service.go` 中始终返回硬编码默认值的 `getJWTSecretFromConfig()` / `getJWTSecretFromEnv()`，统一从 `config.GetJWTSecret()` 读取 (`config/jwt.go`, `config/config.go`)
- **密码时序攻击防护** — 登录时用户不存在也执行 dummy bcrypt 比较，统一返回 `"Invalid credentials"`，防止通过响应时间枚举用户名 (`auth.go`)
- **MFA Token 竞争条件修复** — 新增 `ValidateAndConsumeMFAToken()` 使用数据库事务 + `FOR UPDATE` 行锁原子验证并消费 token，移除非原子的 `MarkMFATokenUsed()` (`mfa_service.go`, `mfa_handler.go`)
- **WebSocket Origin 校验** — 全部 5 个 WebSocket Upgrader 统一使用 `checkWebSocketOrigin()`，通过 `ALLOWED_ORIGINS` 环境变量配置白名单，防止 CSWSH 攻击 (`websocket_handler.go`, `agent_cc_handler.go`, `agent_cc_handler_raw.go`, `agent_metrics_ws_handler.go`, `terraform_output_controller.go`)
- **URL Token 会话固定修复** — 移除 `c.Query("token")` 认证路径，JWT 仅从 Authorization Header 和 WebSocket 子协议获取 (`middleware.go`)
- **租户隔离加固** — workspace/project 级别权限检查不再默认 scope_id 为 `"1"`，缺失时返回 400；`RequireAnyPermission` 中 scope_id 缺失时 `continue` 跳过当前权限（OR 逻辑允许其他权限匹配） (`iam_permission.go`)
- **错误信息泄漏修复** — 中间件 `ErrorHandler` 不再返回内部错误详情，JWT 校验不再暴露 token 类型信息 (`middleware.go`)
- **敏感变量降级阻断** — 禁止将已标记为敏感的变量降级为非敏感 (`workspace_variable_service.go`)

### Features

- **认证端点限速中间件** — 新增 `RateLimiter` 中间件，对登录/MFA 验证等认证端点实施请求频率限制，防止暴力破解 (`rate_limiter.go`, `router.go`)
- **SchemaSolver 重试次数配置化** — 通过环境变量控制 SchemaSolver 最大重试次数 (`config.go`)

### Refactoring

- **日志规范化** — 15 个文件中 102 处 `fmt.Printf` / `fmt.Println` 替换为 `log.Printf`，P1 级别文件（MFA、AI 分析）中敏感信息脱敏处理（token 前缀截断、IP 地址掩码、session ID 移除）
- **Execute 核心流程加固** — 防御性错误处理加固，废弃独立 Apply 死代码清理 (`terraform_executor.go`, `workspace_lifecycle.go`, `task_queue_manager.go`)

### Tests

- **认证限速中间件测试** — `rate_limiter_test.go`
- **SchemaSolver / SkillAssembler 测试覆盖** — `schema_solver_test.go`, `schema_solver_loop_test.go`, `skill_assembler_test.go`
- **Terraform Executor 测试覆盖** — `terraform_executor_test.go`

### Full Changelog

https://github.com/w564791/iac-platform/compare/v0.2.8...v0.2.9
