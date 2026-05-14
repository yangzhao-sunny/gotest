---
title: Build multi-user task management service
domains: [auth, user, project, task, comment, stats, health]
run-id: 2026-05-12-taskmgr
---

# Build multi-user task management service

## 背景

需要构建一个完整的多用户任务管理后端服务，覆盖 Go 后端常见技术维度：JWT 鉴权、关系型数据建模、数据库迁移、cursor 分页、事务、Redis 缓存与限流、OpenAPI 契约、结构化日志、OpenTelemetry 可观测性。项目作为 gkit 技术栈综合测试用例，要求可生产级运行。

## 设计

### 技术栈

- HTTP 框架：Gin
- DB 驱动：pgx v5（直接 SQL）；GORM 仅用于复杂聚合查询（stats）
- 迁移：golang-migrate，独立子命令 `taskmgr migrate up|down`，启动时检查版本但不自动迁移
- 契约：oapi-codegen 生成 server stub + types
- 测试：testcontainers-go 启动真实 Postgres 16 + Redis 7
- 日志：slog JSON handler，结构化字段 request_id / user_id / route / status / latency_ms
- 链路：OpenTelemetry OTLP exporter（可通过 TASKMGR_OTEL_ENABLED=false 关闭）
- 配置：环境变量 `TASKMGR_*` 前缀 + `.env` 文件支持

### 鉴权设计

- Access token：HS256 JWT，TTL 1h，payload 含 user_id / jti
- Refresh token：随机 UUID，存 Postgres `refresh_tokens` 表（含 expires_at / revoked_at）
- Logout：软删 refresh token（设 revoked_at），不失效 access token（TTL 短）
- Auth middleware：从 Authorization header 解析 JWT，注入 user_id 到 gin.Context

### 权限模型

所有受保护资源按 owner_id 隔离；越权统一返回 404，不泄漏存在性。项目所有权通过 `projects.owner_id = $user_id` 过滤；task / comment 通过 JOIN project 间接验权。

### 状态机（Task）

```
todo → doing → done → archived
done → doing  （允许重新打开）
其它跳转 → 409 task_invalid_transition
```

状态转移逻辑集中在 `internal/task/statemachine.go`，handler 调用后持久化。

### 分页

所有列表接口使用 cursor 分页：cursor = base64(created_at::text + "," + id)，按 (created_at DESC, id DESC) 排序，避免 offset 性能问题。

### Redis 用途

1. Stats 缓存：key `stats:{project_id}`，TTL 60s；删除项目时 DEL 对应 key。
2. 评论限流：key `ratelimit:comment:{user_id}:{task_id}`，INCR + EXPIRE 10s；超过 3 次返回 429。

### 异步通知

任务指派时向内部 Go channel 投递 `AssignedEvent{TaskID, AssigneeID}`；后台 goroutine 消费并写 slog INFO 日志，留出后续替换为 Kafka producer 的扩展点（`internal/notify/` 包）。

### 优雅停机

`http.Server.Shutdown` 30s timeout；signal.NotifyContext(SIGTERM, SIGINT)；cleanup 函数关闭 DB pool / Redis 连接。

### 目录结构（预期）

```
cmd/server/          - main.go + buildApp wire
cmd/migrate/         - migrate up|down subcommand
internal/
  auth/              - register, login, refresh, logout, middleware
  user/              - me GET/PATCH
  project/           - CRUD + soft delete
  task/              - CRUD + statemachine
  comment/           - post + list + rate limit
  stats/             - aggregation + cache
  health/            - healthz + readyz
  notify/            - async event channel
  db/                - pgx pool setup + migration check
  redis/             - redis client setup
  middleware/         - request_id, auth, logging, otel
  config/            - env var loading
migrations/          - SQL files (golang-migrate format)
api/                 - openapi.yaml + generated stubs
```

## 数据模型变更预告

新增以下表：`users`、`refresh_tokens`、`projects`（含 deleted_at）、`tasks`（含 deleted_at）、`task_comments`。索引：projects.owner_id、tasks.project_id + status、task_comments.task_id + created_at。SQL 细节由 `/sql-migration` 管理。

## 验收用例

- 新用户完成注册 → 登录 → 创建项目 → 创建任务 → 状态流转到 done → 查询 stats 返回 `{done: 1, todo: 0, doing: 0, archived: 0, overdue: 0}`。
- 用户 A 创建的项目，用户 B 携带合法 JWT 访问 `GET /v1/projects/{id}` 返回 404。
- 同一用户对同一 task 10 秒内提交第 4 条评论，返回 429 错误码 `rate_limited`；等待 11 秒后再次提交返回 201。
- 任务从 `todo` 直接 POST transition 到 `archived`，返回 409 错误码 `task_invalid_transition`。
- 删除项目后：`GET /v1/projects/{id}/tasks` 返回空列表；`GET /v1/projects/{id}/stats` 返回 404；Redis 中对应 stats key 不存在。
- 创建任务时设 `due_date` 为昨天，status 为 `doing`；stats 中 `overdue` 计数为 1。
- 服务进程收到 SIGTERM 后，新建连接被拒；已进入的慢请求（人工注入 sleep）能完成并返回 200；进程在 30s 内退出。
- 停止 Postgres 容器后 `GET /readyz` 返回 503；`GET /healthz` 仍返回 200。
- 使用过期 refresh token 调用 `POST /v1/auth/refresh` 返回 401。
- `PATCH /v1/tasks/{id}` 将 assignee_id 设为另一用户时，slog 输出包含 `assigned` 事件的 INFO 日志行。
