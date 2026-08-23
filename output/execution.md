# gb-526 执行与验收报告

- 项目：`commercial-diving-decompression-control`
- 验收日期：2026-08-22（Asia/Shanghai）
- 实现提交：`9422d54a9ba11ebafb882db091cbe132d079a005`
- 验收方式：本机真实命令、真实 PostgreSQL、真实 HTTP API、Codex 内置 Browser
- 安全边界：仅用于商业潜水训练比较与决策支持；不是医疗器械、认证潜水表或现场生命支持系统，不连接潜水设备，不生成可执行减压指令，不替代主管或医疗专业人员。

## 1. 构建、测试与规模

以下命令均在项目根目录真实执行并返回 0：

| 检查 | 结果 |
| --- | --- |
| `go work sync` | 通过，无输出 |
| `go build ./backend/...` | 通过 |
| `go vet ./backend/...` | 通过 |
| `go test ./backend/...` | 通过；`constants`、`decompression` 测试通过 |
| `go test -race ./backend/...` | 通过；未发现竞态 |
| `npm --prefix frontend ci` | 通过；按 lockfile 安装 143 个包 |
| `npm --prefix frontend run typecheck` | 通过 |
| `npm --prefix frontend run build` | 通过；Vite 生产构建完成 |
| `project_scale.py .` | `3245` 行、`42` 个功能 `.go` 文件，检测到前端 |
| `runtime_smoke.py .` | 通过；`http://127.0.0.1:20526/healthz` 返回 HTTP 200 |

规模满足提示词的 `2500-4200` 行、`24-42` 个功能 Go 文件且低于 5000 行红线。规模脚本不计测试、vendor、生成代码和前端。

前端构建仅报告 ECharts 页面 chunk 超过 500 kB 的性能提示；没有类型或构建错误，不阻断本轮功能验收。

## 2. Compose 与运行态

真实执行：

1. `docker compose config --quiet`
2. `docker compose build`
3. `docker compose up -d`
4. `docker compose ps`
5. 访问前端、`/healthz` 和 `/readyz`

运行时最终状态：

| 服务 | 端口 | 状态 |
| --- | --- | --- |
| frontend | `18526 -> 80` | `healthy`，首页 HTTP 200 |
| backend | `19526 -> 8080` | `healthy`，`/healthz` HTTP 200 |
| db | `57526 -> 5432` | `healthy`，真实 PostgreSQL 16 |

最终健康响应：

```json
{"data":{"status":"ok"},"request_id":"0c4ed129-8b61-4e0c-ba6d-c38aa5e1a8fd"}
{"data":{"database":"postgres","model_version":"training-compartment-v1","status":"ready"},"request_id":"55727df6-c5b8-42c7-a49e-d1a4b1af1431"}
```

## 3. API 验收

共完成 22 项主链检查和 9 项补充边界断言。所有调用使用真实后端和 PostgreSQL，没有 mock。关键结果如下：

| 场景 | HTTP/结果 | request ID |
| --- | --- | --- |
| 匿名访问受保护资源 | `401 AUTH_REQUIRED` | `60fbc1cc-5420-407e-9c08-e9b2a263ef72` |
| planner 创建训练档案 | `201` | `22b4ce8d-741e-4ef5-bd6f-ae5e78c9eb0e` |
| planner 创建计划 | `201` | `dbf4a855-d551-4b96-92a0-612b043477a9` |
| 四个暴露段创建 | 均成功且写入审计 | `bf8390de-c471-43b2-b4a0-523278aebdf4`、`c43aa91f-6515-4c1e-83d7-b5e3fddab417`、`4d0f0743-f50a-49e2-af5a-a80dce9e057f`、`780f6977-12d8-4add-b6bf-c65f569c6f69` |
| 气体组分和非法 | `422 INVALID_GAS_MIX`，未写入 | `b644b563-5a46-447d-b93b-9f11ab7e946d` |
| 暴露段排序 | 成功、版本推进 | `5eae3e3c-0d0c-4a64-91cd-f7c498842f9d` |
| 恢复有效顺序 | 成功、版本再次推进 | `bab6cf43-8aef-4584-af38-dec2ccfa7df6` |
| 运行确定性模型 | `201`，创建不可覆盖 assessment `#2` | `88e10c04-b5dc-4151-9ff8-0997f17468c8` |
| 按 ID 重放输入快照/输出 | `200`，快照、六舱结果与假设完整 | `65217bff-1971-45a4-acf8-362282abc610` |
| planner 提交人工复核 | 成功，状态进入 `pending_supervisor_review` | `cb7442ca-0513-417e-b1e6-4860607749ab` |
| planner 越级批准 | `403 FORBIDDEN` | `bd4543b5-8376-4b3a-ba37-1e5d13eb2467` |
| supervisor 尝试计划员写操作 | `403 FORBIDDEN` | `e05d2cde-95f1-46fd-8a40-358ca9f9280e` |
| supervisor 批准训练用途 | 成功，计划与评估原子迁移 | `6333a24e-53f7-4a4f-b36e-faaf9a09f4eb` |
| 重复批准 | `422 INVALID_PLAN_TRANSITION` | `cd46dcb0-8f3a-4034-994f-8d4031c22150` |
| 比较两份评估 | `200`，返回相对差异而非安全结论 | `de43f18e-c334-4dfd-bd9f-6f89478ed4cb` |
| supervisor 查询审计 | `200`，包含 request ID、操作者和前后摘要 | `ce25a043-fc64-4ad1-8e07-d1e96ce80484` |
| 档案旧版本更新 | `409 PROFILE_VERSION_CONFLICT` | `240739fa-33f2-4a50-9607-d0a3be19708a` |
| 段序号存在缺口时运行模型 | `422 MODEL_INPUT_INVALID` | `9c574a72-7915-4c73-84e1-34df3bfd7d51` |
| 失败建模后读取计划 | `200`，仍为 `draft` | `67070022-56cb-4e79-8eb9-aa2b7d461354` |
| 失败建模后查询评估 | `200`，该计划无评估记录 | `b75b9b79-3647-457f-acff-b7069029dbf6` |

主 API 脚本在第 12 项后曾把统一响应中的 `data` 数组误判成根数组，导致本地 `jq` 断言中止；对应接口真实返回 HTTP 200。修正断言后，验收从已核对的数据库状态继续完成第 13-22 项。这是验收脚本问题，不是应用接口故障。

镜像重建会轮换容器 stdout，因此最终复核同时读取了持久化审计表。数据库中存在 26 条审计事件，写操作 request ID 与上述创建、排序、建模、提交和批准请求一致。

## 4. 数据库约束与最终数据

真实查询 PostgreSQL `pg_constraint`，确认以下约束存在：

- `chk_dive_plans_plan_status`
- `chk_decompression_assessments_assessment_status`
- `chk_decompression_assessments_highest_risk_band`

验收结束前的业务数据计数：

| 表 | 行数 |
| --- | ---: |
| `diver_profiles` | 3 |
| `dive_plans` | 4 |
| `exposure_segments` | 13 |
| `decompression_assessments` | 3 |
| `audit_events` | 26 |

关键终态：`TRAIN-30A` 与 `QA-PLAN-526` 为 `approved_for_training`；`COMPARE-42B` 为 `pending_supervisor_review`；序号缺口用例 `QA-GAP-526` 保持 `draft` 且没有生成评估。三份历史评估的 `highest_risk_band` 均满足数据库约束。

## 5. Codex 内置 Browser 验收

仅使用 Codex 内置 Browser，没有调用外部 Chrome 或 Computer Use。

1. 使用 planner 登录，守卫正常；`/divers` 展示 3 个档案，`QA-DIVER` 可查看 2 个关联计划。
2. `/plans` 选择 `TRAIN-30A`，真实执行 Move later、Move earlier 并恢复顺序，审计留下两次 reorder；运行模型后从 `draft` 进入 `modeled`（v4）。
3. 镜像重建后执行强制 reload；`/exposures` 通过 MUI 下拉选择 `TRAIN-30A`，ECharts canvas 非空，展示深度曲线、6 条组织舱曲线、4 段 ledger、模型假设与安全边界。
4. `/assessments` 展示 assessment `#3`、比较指数 `82`、最高风险 `elevated` 和 2 条风险证据；planner 提交后进入 `SUPERVISOR REVIEW`。
5. supervisor 登录并执行 Approve training，终态为 `TRAINING APPROVED`，页面明确说明这不构成 operational clearance。
6. `/audit` 首部显示 supervisor 的两条批准审计、before/after 状态与相同 request ID，同时可见模型运行和两次排序事件。
7. 最终 Sign out 返回 `/login`；最新镜像全程 `dev.logs() = []`，桌面目视未见重叠或截断。

补充回归：随后使用 Codex 内置 Browser 的 viewport 能力以 `390 x 844` 重新打开 `/divers`、`/plans`、`/exposures` 和 `/assessments`。四页均加载真实 API 数据，`document.documentElement.clientWidth=390`、`scrollWidth=390`，未出现页面级横向溢出；登录后的路由切换和退出路径正常，`tab.dev.logs()` 返回 `[]`。移动端截图：`output/browser-mobile-390.png`。

截图均为真实 Browser 产物：

- `output/root-browser-divers.jpg`（1280x759）
- `output/root-browser-plan-modeled.jpg`（1280x872）
- `output/root-browser-exposure-model.jpg`（1280x1187）
- `output/root-browser-assessment-pending.jpg`（1280x1387）
- `output/root-browser-assessment-approved.jpg`（1280x1387）
- `output/root-browser-audit.jpg`（1280x2094）

## 6. 安全与质量结论

- 页面和 API 结果持续标记 training/decision-support 边界，不输出“安全可潜”或可执行停留深度/时长。
- JWT/RBAC 覆盖 planner、supervisor、admin；后端路由和前端按钮显隐均参与验收。
- 气体、序列、连续性、深度、时长和上升速率错误会阻止计算；失败不会在前端显示为通过。
- 评估保存输入快照、算法版本、假设、六舱曲线和风险证据且不可覆盖；状态迁移使用事务、版本条件和审计理由。
- README 包含 Docker 优先启动、账号、功能、技术栈、目录、环境变量、API、枚举位置、模型假设、安全边界、开发命令、排错和 License。

## 7. 清理

验收完成后真实执行：

```bash
docker compose down -v --remove-orphans
```

随后分别检查 `docker compose ps --all`、项目 Compose label、命名卷和项目网络；本项目容器、`commercial-diving-decompression-control-postgres-data` 卷及 `commercial-diving-decompression-control_default` 网络均为空。未停止或删除其他项目资源。
