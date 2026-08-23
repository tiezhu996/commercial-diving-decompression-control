# DiveSafe 减压暴露校核台：企业级复审

审计日期：2026-08-22  
项目：`commercial-diving-decompression-control`（gb-526）

## 结论

通过企业级复审。`output/execution.md` 已真实覆盖 Go race/build/vet/test、前端构建、真实 PostgreSQL Compose、22 项 API 链路、Codex 内置 Browser 的训练计划/建模/复核/审计交互、停服和安全边界。本次补强认证状态一致性。

## 发现与修复

首轮中 JWT claims 携带 username/role，middleware 只使用签名 claims；账号被停用或降权后旧 token 仍可继续操作。现已在 `backend/internal/auth/auth.go` 增加 `FindByID` 和 `CurrentPrincipal`，在 `backend/internal/middleware/auth.go` 每次请求重新查询用户，拒绝不存在/停用账号，并使用数据库中的最新 username/role；`backend/internal/auth/auth_test.go` 覆盖角色变更和停用即时生效。

## 验证

```text
gofmt -w backend/internal/auth/auth.go backend/internal/auth/auth_test.go backend/internal/middleware/auth.go 通过
go test ./backend/... 通过
go vet ./backend/... 通过
```

Compose、API 和内置 Browser 既有证据见 `output/execution.md`；本次未重新启动容器或浏览器。仓库提交后保持 clean。

## Git

本次修复提交：`fix: revalidate current user role on protected requests`。身份：`blueship581 <brysj.hhrhl.g@gmail.com>`。
