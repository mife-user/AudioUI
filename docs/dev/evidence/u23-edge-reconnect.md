# U23 连线箭头拖拽及关系浮层自检记录

## 状态

实现、代码级自动化与离线构建自检已完成。用户于 2026-11-15 确认 AC-U-60～62 页面验收通过。具体验收步骤与截图未归档；助手未启动或操控界面。U24 顶部入口清理和关系栏抖动收口未实施。

## 实现

- 普通边目标端保持原有三角箭头；箭头附近叠加不可见的 24px 屏幕尺寸拖拽命中区，不绘制圆环或额外箭头装饰。同一输入端汇入多条可改接边时，命中区按稳定边 ID 错开；悬停时高亮对应连线。候选来源边、运行锁边和当前不可操作的边不提供拖拽命中区。
- 拖动预览复用现有避障路由。落点按真实 React Flow target Handle 的屏幕范围识别；即使默认 CSS 使 Handle 不参与 `elementFromPoint()` 命中，也能找到端口。节点卡片空白处不作为落点；拖回原目标、取消或无效落点不改图。
- 拖动开始时冻结工程 ID 和完整旧边身份；提交前重验并调用 U22 `ReconnectTarget`。收到 mutation receipt 后等待对应权威快照，不乐观改图；拒绝保留原边，同步失败沿用仅重试快照语义。
- 点击线身或聚焦后按 Enter/Space 可打开局部关系浮层。普通边提供确认断开；Script 来源边可逐边设置固定版本；候选来源只读。浮层不提供改挂选择，不占用画布布局。旧顶部入口暂留，供 U24 收口。

## 自动化与离线检查

工作目录：`D:\vscode\AI\bilibili\AudioUI`。测试使用本地夹具，不访问网络、不调用真实 Agent/CLI、不操作用户工程。

| 检查 | 结果 |
|---|---|
| 2 个画布定向测试文件 | 51 项通过；真实 React Flow DOM 回归包含新 Agent/新提示词目标 Handle（`pointer-events:none` 背景下按端口屏幕范围命中） |
| `npm --prefix frontend run test` | 24 个测试文件、245 项通过 |
| `npm --prefix frontend run typecheck` | 通过 |
| `npm --prefix frontend run lint` | 0 错误；2 条既存 `react-refresh/only-export-components` 警告，位于 `usePendingController.tsx` |
| `npm --prefix frontend run check:tokens` | 通过 |
| `GOPROXY=off GOSUMDB=off go vet . ./internal/...` | 通过 |
| `GOPROXY=off GOSUMDB=off go test . ./internal/...` | 全部通过 |
| `wails3 generate bindings -ts -i` | 离线生成成功；未手工编辑生成文件 |
| `npm_config_offline=true npm --prefix frontend run build` | 通过；构建产物 JS 608.60 kB、gzip 185.89 kB，Vite 提示单个 minified chunk 大于 500 kB |
| `GOPROXY=off GOSUMDB=off npm_config_offline=true wails3 build` | 离线构建成功，生成 `bin/audio-ui.exe` |
| `bin/audio-ui.exe --version` | 输出 `AudioUI v0.2.0` |

## AC-U-60～62 自检映射

| 验收项 | 代码级覆盖与未覆盖边界 |
|---|---|
| AC-U-60 | DOM 回归覆盖目标 Handle 落点、空白/原目标、指针取消、Escape、后端拒绝；拖动期间边身份变化和工程切换不会把旧意图应用到新身份。用户于 2026-11-15 确认 AC-U-60～62 页面验收通过；具体验收步骤未归档。 |
| AC-U-61 | 覆盖共享输入的抓手错开，以及候选来源和运行锁保护；实际 Handle 命中限制避免落在节点空白处。用户于 2026-11-15 确认 AC-U-60～62 页面验收通过；具体 DPI/缩放操作记录未归档。 |
| AC-U-62 | DOM 回归覆盖鼠标与键盘打开浮层、Script 固定版本、确认断开；候选来源不提供写操作。用户于 2026-11-15 确认 AC-U-60～62 页面验收通过；详细键鼠步骤与焦点记录未归档。 |

## 用户页面验收

用户于 2026-11-15 在 `D:\vscode\AI\bilibili\AudioUI\bin\audio-ui.exe` 中确认 AC-U-60～62 页面验收通过。助手未启动或操控界面；逐项操作步骤与截图未归档。
