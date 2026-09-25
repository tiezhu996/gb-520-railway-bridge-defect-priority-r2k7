# 验收记录

验收日期：2026-09-25（限速定稿联动与受限恢复守卫补充验收）

## 限速定稿联动 / 受限恢复守卫

- 独立 reviewer 将 `K42 桥梁作业区` 的草稿定稿为 urgent/restrict 后，同设施桥梁 BA-004 在同一事务内由 active 进入 restricted，version 自增，并产生 BridgeAsset active→restricted 审计记录。
- 同设施桥梁为 closed（封闭停用区域5 BA-005）/retired 时定稿 restrict 返回 422 `business_rule`；找不到同设施桥梁时同样 422；定稿被拒绝后决定仍停留在 draft。
- observe 定稿不改变桥梁状态（BA-001 保持 active、version=1）。
- 已受限桥梁上再次定稿 restrict/urgent 时桥梁保持 restricted，不重复自增。
- `restricted → active`：operator 返回 403；reviewer/admin 在同设施存在 new/verified 缺陷时返回 422 `unconfirmed_defects`，信息写明剩余条数（如 `1 defect(s) ...`）；缺陷推进到 monitoring/mitigated、未确认条数清零后复核恢复成功。
- `/api/bridges` 列表与 `/api/bridges/:id` 详情均返回 `unconfirmedDefectCount`。
- 服务层新增跨聚合测试见 `backend/internal/service/bridge_linking_test.go`；`go test ./...`、`go test -race ./...`、`go vet ./...`、`go build ./...` 均通过；前端 `npm run typecheck`、`npm run build` 均通过。

## 静态质量

- `go test ./...`：通过。
- `go test -race ./...`：通过。
- `go vet ./...`：通过。
- `go build ./...`：通过。
- `npm run typecheck`：通过。
- `npm run build`：通过。Vite 仅报告主包大于 500 kB 的性能提示，不影响构建与运行。
- 非测试 Go 代码：3147 行、38 个 `.go` 文件，符合提示词 3000–4200 行、30–42 文件要求。

## 空卷 Compose 与 API

执行 `KEEP_RUNNING=1 ./scripts/validate.sh` 前，脚本已运行 `docker compose down -v --remove-orphans`。PostgreSQL、Redis、MinIO、后端和前端均从空卷启动并通过健康检查。

- viewer 可以读取业务数据；写接口与审计接口均返回 403。
- operator 创建优先级 v1 并补充证据形成 v2；直接定稿返回 403。
- reviewer 自己拟制后自行复核返回 422。
- 独立 reviewer 将 operator 草稿定为 urgent v3。
- v1/v2/v3 的证据、actor 和 request ID 均按原值保留。
- 终态后再次编辑返回 422。
- 审计汇总包含 create、update 和 transition。

## 内置 Browser

仅使用 Codex 内置 Browser 验证，没有调用外部 Chrome 或独立 Playwright。

- viewer 登录后看不到新增、推进和审计导航；直接访问 `/audit` 被守卫重定向到 `/bridges`。
- operator 在 `/defects` 创建缺陷并从 new 推进到 verified；`SeverityBadge` 和证据摘要正常显示。
- `/inspections` 与 `/defects` 均渲染共享 `EvidenceGallery`。
- operator 在 `/priorities` 看不到定稿按钮；reviewer 可对他人拟制的 PD-001 定稿，自己拟制的草稿无操作按钮。
- `/audit` 正确显示操作者、状态迁移和 request ID。
- `/bridges`、`/inspections`、`/defects`、`/priorities`、`/audit` 在 390×844 下的 `innerWidth`、`body.scrollWidth`、`documentElement.scrollWidth` 均为 390。
- 浏览器控制台 error/warning：0。

默认 `./scripts/validate.sh` 会在结束时执行 `docker compose down -v --remove-orphans`，不会保留本项目容器、网络或命名卷。
