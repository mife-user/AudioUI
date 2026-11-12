# U13 节点折叠与 Topic 候选呈现自检

## 实现范围

- 五类节点的 `collapsed` 状态写入工程；`SetNodeCollapsed` 使用工程作用域回执和 revision 保存，允许在运行锁期间折叠/展开。
- 折叠只将显示高度切为 56 画布像素，不改保存的展开尺寸；折叠态不提供缩放手柄或尺寸对话框，仍可移动。
- 正文用 `hidden` 隐藏但保持组件挂载，保留 Topic/Prompt 草稿及节点内部状态。标题区保留类型、标题、运行状态、请求/待处理入口与折叠控制。
- Topic 候选 Handle 保留候选索引 ID，统一固定在标题区可见锚点；每条边仍指向快照中的真实 Candidate。标题区显示候选数/读取状态，正文列表可滚动并定位真实 Candidate。
- 折叠/展开状态变化触发 React Flow Handle 重新测量；折叠尺寸用于当前边路由，节点自动避让仍按保存的展开尺寸计算。

## 自动化测试与构建

| 检查 | 结果 |
|---|---|
| `go test ./internal/domain/entity ./internal/service/workflow ./internal/handler ./internal/infra/persistence` | 通过 |
| `go test . ./internal/...` | 全部通过 |
| `go vet . ./internal/...` | 通过 |
| `npm --prefix frontend run typecheck` | 通过 |
| `npm --prefix frontend run lint` | 0 错误；`usePendingController.tsx:54,248` 有 2 条既有 Fast Refresh 警告 |
| `npm --prefix frontend run check:tokens` | 通过 |
| `npm --prefix frontend run build` | 通过；Vite 提示 JS chunk 584.32 kB，超过 500 kB 提示阈值 |
| CanvasNodes U13 定向测试 | 7 项通过；覆盖五类节点折叠/展开、十候选固定锚点及候选定位入口 |
| WorkspaceCanvas React Flow 定向测试 | 4 项通过；覆盖实测 Handle/边、草稿保留、折叠/展开、Candidate 定位及候选读取失败 |
| `flowModel.test.ts` + `nodeState.test.ts` | 18 项通过 |
| Wails bindings（`-clean=false`） | 生成成功 |
| 离线 Wails 构建 | `GOPROXY=off GOSUMDB=off npm_config_offline=true wails3 build` 成功，生成 `bin/audio-ui.exe` |
| `git diff --check` | 通过；Git 提示部分现有工作树文件将以 CRLF 写回 |

前端完整 Vitest 套件未能完成：单独运行完整 `CanvasNodes.test.tsx` 时，worker 在约 4 GiB 堆上限处 OOM（结束前 21/24 项通过）；完整套件单 worker 运行时 worker 意外退出，未返回测试结果。以上不计作完整前端测试通过；U13 定向测试单独通过。

## 尚未覆盖

- 实际 Wails/WebView2 界面和手工验收尚未执行；由用户使用本次重建的 `bin/audio-ui.exe` 验收。
- 本次未启动浏览器或桌面窗口，未调用真实 Agent，也未下载依赖。Wails 构建期间 npm 使用离线模式并报告依赖已是最新。
