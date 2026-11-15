# U25 选中节点四角外观自检与验收

| 项目 | 结果 |
|---|---|
| 状态 | 用户于 2026-11-15 确认 AC-U-66～67 页面验收通过 |
| 范围 | AC-U-66～67；回归 AC-U-05～07、AC-U-23 的相关能力；U12 历史验收不回改 |
| 视觉依据 | [U25 调整前后单文件示意](../../design/u25-selected-node-outline-demo.html)，仅为静态模拟 |

## 实现

- `workspace.css` 将四角手柄的边框和背景设为透明、阴影设为无；保留 10px 宽高、1px 边框占位和原圆角，不隐藏元素或禁用指针。
- 未改变 `NodeResizer` 的四边四角控件及其可见条件，也未改变现有选中节点边线、光晕、连接端口、尺寸对话框、折叠规则、后端或工程格式。

## 自动化与构建结果

| 检查 | 结果 |
|---|---|
| Go 全量 `go vet . ./internal/...`、`go test . ./internal/...` | 通过；设置 `GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off`。 |
| 前端全量 `npm --prefix frontend run test` | 24 个测试文件、258 项通过；包括五类节点的样式和控制结构、折叠/禁用、尺寸设置与焦点，以及 React Flow DOM 八向模拟拖动的锚点与布局提交。 |
| `typecheck`、`check:tokens` | 通过。 |
| `lint` | 0 错误；`usePendingController.tsx` 两条既存 Fast Refresh 警告。 |
| 前端 `npm --prefix frontend run build` | 通过；Vite 保留已有的单个 chunk 超过 500 KB 提示。 |
| `wails3 generate bindings -ts -i -clean=true` | 通过；重新生成前后 `frontend/bindings`、`go.mod`、`go.sum` 与 `package-lock.json` 无差异。 |
| `wails3 build` | 通过；离线完成绑定生成、前端生产构建、图标与 syso 生成及 Go 链接，生成 `bin/audio-ui.exe`；构建后无残留 `.syso`，依赖与生成文件无差异。 |
| `bin/audio-ui.exe --version` | 输出 `AudioUI v0.2.0`；产物修改时间为 2026-11-15 19:11。 |
| 真实页面 / Windows WebView2 | 助手未执行；用户使用本次构建的 exe 完成页面验收。 |

本次没有启动浏览器、Wails 窗口或真实 Agent，也没有联网下载。

## 离线构建命令

用户授权离线构建后，在 `D:\vscode\AI\bilibili\AudioUI` 使用已提交的生成文件状态（`frontend/bindings` 无未提交改动、无待保留的 `.syso`）执行：

```bash
export GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off
export npm_config_offline=true npm_config_audit=false npm_config_fund=false
wails3 generate bindings -ts -i -clean=true
wails3 build
```

构建任务内部的 `npm install` 报告 `up to date`，未下载依赖。

## AC 覆盖与未覆盖项

| AC | 自动化/构建证据 | 用户页面验收 |
|---|---|---|
| AC-U-66 | 五类节点的 React Flow DOM 计算样式测试确认四角无边框、背景和阴影，手柄数量、尺寸、四边防缩线、连接端口及选中边线样式保留；折叠、禁用与未选中状态回归。 | 用户于 2026-11-15 确认验收通过。 |
| AC-U-67 | 八向模拟拖动的缩放锚点与布局提交测试；尺寸对话框、恢复默认、焦点及运行锁路径测试；离线 exe 构建通过。 | 用户于 2026-11-15 确认验收通过。 |

用户于 2026-11-15 确认 AC-U-66～67 页面验收通过；页面操作步骤与截图未归档。助手未启动或操控浏览器/Wails 窗口，未执行真实 Agent、CLI 或网络访问。自动化、离线构建与用户页面验收分别记录，自动化结果不替代页面验收。
