package app

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"sync/atomic"

	"audio-ui/internal/handler"
	"audio-ui/internal/infra/environment"
	"audio-ui/internal/infra/logging"
	"audio-ui/internal/infra/paths"
	"audio-ui/internal/infra/winmsg"
	"audio-ui/internal/service/project"
	"audio-ui/internal/service/system"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"go.uber.org/zap"
)

const (
	appName        = "AudioUI"
	singleInstance = "com.audioui.desktop"
	webView2URL    = "https://developer.microsoft.com/microsoft-edge/webview2/"
)

// Config is supplied by the thin root entry point.
type Config struct {
	Assets  fs.FS
	Version string
}

// Run starts the GUI or one of the headless diagnostic modes.
func Run(cfg Config) int {
	mode := parseMode(os.Args[1:])
	switch mode {
	case modeVersion:
		fmt.Printf("AudioUI v%s\n", cfg.Version)
		return 0
	case modeHelp:
		printHelp()
		return 0
	}

	layout, err := paths.NewResolver().Resolve()
	if err != nil {
		fatal(mode, "AudioUI 启动失败", err.Error())
		return 1
	}
	if err := paths.Ensure(layout); err != nil {
		fatal(mode, "AudioUI 启动失败", err.Error())
		return 1
	}

	logger, err := logging.New(layout.LogFile)
	if err != nil {
		fatal(mode, "AudioUI 启动失败", "无法初始化日志: "+err.Error())
		return 1
	}
	defer logger.Close()

	logger.Zap.Info("audio-ui starting",
		zap.String("version", cfg.Version),
		zap.String("root", layout.Root),
		zap.String("root_source", layout.RootSource),
	)

	cliProbe := environment.NewClaudeProbe()
	webviewProbe := environment.NewWebView2Probe()

	if mode == modeSelfCheck {
		return runSelfCheck(context.Background(), layout, cfg.Version, cliProbe, webviewProbe, logger)
	}

	webviewStatus := webviewProbe.Detect(context.Background())
	if !webviewStatus.Installed {
		message := "未找到 WebView2 运行时。\n\n请安装 Microsoft Edge WebView2 Runtime 后重试：\n" + webView2URL
		logger.Zap.Error("webview2 runtime missing", zap.String("detail", webviewStatus.Error))
		winmsg.Error("AudioUI 无法启动", message)
		return 1
	}

	systemService := system.New(layout, cfg.Version, cliProbe, webviewProbe, logger.Tailer)
	systemHandler := handler.NewSystemService(systemService)

	bus := newWailsEventBus()
	workspaceCoordinator := newWorkspaceCoordinator(bus)
	projectSetup := newProjectSetup(layout, logger.Zap, workspaceCoordinator.ProjectChanged)
	workflowSetup := newWorkflowSetup(layout, projectSetup.service, workspaceCoordinator.WorkflowChanged)
	if activeProject, err := projectSetup.service.Current(context.Background()); err == nil {
		workspaceCoordinator.Initialize(activeProject.ID, activeProject.Revision)
	} else {
		workspaceCoordinator.Initialize("", 0)
		if !errors.Is(err, project.ErrNoActiveProject) {
			logger.Zap.Warn("workspace initial project unavailable", zap.Error(err))
		}
	}
	agentSetup := newAgentSetup(layout)
	runSetup, err := newRunSetup(layout, cfg.Version, logger.Zap, bus, projectSetup.service, workflowSetup.service)
	if err != nil {
		logger.Zap.Error("run setup failed", zap.Error(err))
		winmsg.Error("AudioUI 启动失败", "运行管理初始化失败: "+err.Error())
		return 1
	}
	workspaceHandler := handler.NewWorkspaceService(workflowSetup.service, runSetup.manager, workspaceCoordinator)

	var mainWindow atomic.Pointer[application.WebviewWindow]
	var pendingFocus atomic.Bool

	app := application.New(application.Options{
		Name:        appName,
		Description: "本地运行的节点化内容生产工作台",
		Services: []application.Service{
			application.NewService(systemHandler),
			application.NewService(projectSetup.handler),
			application.NewService(workflowSetup.handler),
			application.NewService(agentSetup.handler),
			application.NewService(runSetup.handler),
			application.NewService(workspaceHandler),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(cfg.Assets),
		},
		Windows: application.WindowsOptions{
			WebviewUserDataPath: layout.WebView2Dir,
		},
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: singleInstance,
			OnSecondInstanceLaunch: func(_ application.SecondInstanceData) {
				if window := mainWindow.Load(); window != nil {
					window.Restore()
					window.Show()
					window.Focus()
					return
				}
				pendingFocus.Store(true)
			},
		},
		ErrorHandler: func(err error) {
			logger.Zap.Error("wails error", zap.Error(err))
			winmsg.Error("AudioUI 运行错误", err.Error())
		},
		OnShutdown: func() {
			runSetup.manager.Close()
			agentSetup.session.Close()
			if runSetup.audit != nil {
				_ = runSetup.audit.Close()
			}
		},
	})

	window := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:             "main",
		Title:            appName,
		Width:            1440,
		Height:           900,
		MinWidth:         1024,
		MinHeight:        700,
		InitialPosition:  application.WindowCentered,
		BackgroundColour: application.NewRGB(10, 14, 17),
		URL:              "/",
	})
	mainWindow.Store(window)
	bus.SetApp(app)
	app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		bus.Start()
		if pendingFocus.Swap(false) {
			window.Restore()
			window.Show()
			window.Focus()
		}
	})

	logger.Zap.Info("main window created", zap.Int("width", 1440), zap.Int("height", 900))
	if err := app.Run(); err != nil {
		logger.Zap.Error("application exited", zap.Error(err))
		return 1
	}
	return 0
}

func fatal(mode runMode, title, message string) {
	if mode == modeSelfCheck {
		fmt.Fprintf(os.Stderr, "%s: %s\n", title, message)
		return
	}
	winmsg.Error(title, message)
}
