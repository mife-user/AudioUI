# U20 点阵画布与轻微动效自检

## 状态

用户于 2026-11-14 确认 AC-U-51 验收通过；详细页面操作记录和截图未提供，本证据不补写。助手未启动或操控浏览器、Wails 窗口。

## 实现

- 活动画布只使用 React Flow 的 22px SVG 点阵，`size=2` 使点在 1x 缩放时直径约 2px。React Flow 背景 SVG 默认位于 `z-index: -1`，会落到工作区底色下；现将这个既有背景提升到 `z-index: 0`，保持画布容器透明，节点平面仍在其上方。固定 CSS 点阵仅用于加载、错误和无工程等占位状态。
- 点阵颜色沿用 `--grid` 令牌。正常模式仅对单个 SVG 点形状做 10 秒、低幅度透明度渐变（0.85～1）；`prefers-reduced-motion: reduce` 时关闭动画并保持静态。
- 动画不创建逐点 DOM，不使用 JavaScript 帧循环，不改变视口、节点、连线、命中区或保存行为。

## 自动化与构建

工作目录：`D:\vscode\AI\bilibili\AudioUI`。

| 检查 | 结果 |
|---|---|
| `npm --prefix frontend run test -- src/ui/WorkspacePage.test.tsx src/canvas/WorkspaceCanvas.test.tsx src/canvas/WorkspaceCanvas.realFlow.test.tsx` | 3 个测试文件、61 项通过；工作区测试确认活动画布不使用固定底纹，React Flow DOM 覆盖唯一 SVG 点阵、`userSpaceOnUse` 及缩放后 pattern 间距变化 |
| `npm --prefix frontend run test` | 24 个测试文件、236 项通过 |
| `npm --prefix frontend run typecheck` | 通过 |
| `npm --prefix frontend run lint` | 0 错误；2 条既存 `react-refresh/only-export-components` 警告，位于 `usePendingController.tsx` |
| `npm --prefix frontend run check:tokens` | 通过 |
| `npm --prefix frontend run build` | 通过；首屏 JS gzip 183.48 kB；Vite 提示未压缩 JS chunk 600.36 kB，超过 500 kB 提示线 |
| `GOPROXY=off GOSUMDB=off npm_config_offline=true wails3 build` | 从项目根目录离线构建通过，生成 `bin/audio-ui.exe`；构建所运行的 npm 依赖步骤报告依赖已最新，锁文件无变化 |
| `bin/audio-ui.exe --version` | 输出 `AudioUI v0.2.0` |
| `git diff --check` | 通过 |

## 未覆盖与用户验收

- 用户已完成实际页面验收；助手按项目约定未启动或操控浏览器/Wails 窗口，详细操作记录和截图未提供。
- 定向 React Flow DOM 测试覆盖 SVG 点阵、z-index、点径与缩放后 pattern 间距变化；jsdom 无法可靠合成画布平移事件，用户已按 AC-U-51 完成页面验收。
- 用户已确认 AC-U-51 验收通过；具体各项操作结果未提供，不补写。
