# U9 受控集成 fixture 与门禁记录

## 状态

实现与自动化/离线构建自检完成，用户已确认最终页面验收通过。此记录不表示 AC-U-18 已通过。

## 受控 fixture

- 入口：`frontend/src/dev/U9IntegrationFixture.tsx`，仅当 Vite 开发模式且查询参数为 `?fixture=u9` 时挂载；生产构建不包含 fixture 标识。
- 覆盖：内存工程、Topic/Candidate/Prompt/Agent/Script 图、运行历史与事件、权限/问题请求、待确认 Script 预览与追加、工作区失败、运行失败、待确认正文读取/追加失败及权限响应失败。
- 所有图命令、运行、pending 和同步均使用本地替身；不调用真实 Wails 工程/运行 API，不启动 Agent，不读取用户工程，不访问网络。
- fixture 复用现有 `WorkspacePage`、`WorkspaceCanvas`、`AgentConsole`、`PendingArtifactCard`、运行/工作区 store 与待处理控制器；它不是静态 HTML 原型，也不作为验收证据。

## 自动化与离线门禁

| 检查 | 结果 |
|---|---|
| Wails TypeScript bindings，`GOPROXY=off GOTOOLCHAIN=local wails3 generate bindings -ts -i` | 通过；无 API 变更，生成内容未造成额外工作树差异。清理式生成被权限策略拒绝，改用不清理的安全方式完成。 |
| `go vet . ./internal/...` | 通过 |
| `go test . ./internal/...` | 通过 |
| 前端 typecheck | 通过 |
| 前端 lint | 0 错误；2 条既存 `usePendingController.tsx` Fast Refresh 警告 |
| 前端 token 检查 | 通过 |
| 前端全量测试 | 25 个测试文件、263 项通过 |
| 前端生产构建 | 通过；JS chunk 604.56 kB，仍有超过 500 kB 的既存体积提示 |
| 生产包隔离 | `frontend/dist` 未检出 fixture 工程 ID、U9 标题或 pending 标识 |
| Wails 离线构建 | 通过，生成 `bin/audio-ui.exe`；`GOPROXY=off`、`GOTOOLCHAIN=local`、`npm_config_offline=true` |
| `git diff --check` | 通过；仅有工作树既有 CRLF 转换提醒 |

## 未覆盖与验收边界

- 助手未启动 Vite 页面、浏览器或 Wails 窗口。用户已确认 U9 最终页面验收通过；未提供逐项交互记录、截图或 viewport，故本记录不补写具体页面观察。
- 未启动真实 CLI/Agent，未运行 `WebSearch`，未访问外网。AC-U-18 继续保持未验收。
- 用户已确认 U9 最终页面验收通过；本记录不补写未提供的具体操作、截图或 viewport。自动化、绑定生成及构建结果仍单独记录，不替代或扩展 AC-U-18。
- 本次只复用现有前端 API 注入点，无新增后端 API、领域行为或架构决策，因此未新增 ADR。

## 本地使用

在仓库根目录运行 `npm --prefix frontend run dev`，然后由用户打开 Vite 输出的本机地址并附加 `?fixture=u9`。Vite 默认绑定 `127.0.0.1:9245`；fixture 仅存在于开发服务器，桌面生产 exe 不启用该入口。
