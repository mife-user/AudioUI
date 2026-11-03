package app

import (
	"context"
	"fmt"
	"os"
	"time"

	"audio-ui/internal/domain/port"
	"audio-ui/internal/domain/valueobject"
	"audio-ui/internal/handler"
	"audio-ui/internal/infra/agent/claudecli"
	"audio-ui/internal/infra/audit"
	"audio-ui/internal/infra/paths"
	"audio-ui/internal/infra/persistence"
	"audio-ui/internal/infra/workspace"
	contextservice "audio-ui/internal/service/context"
	"audio-ui/internal/service/permission"
	"audio-ui/internal/service/project"
	runservice "audio-ui/internal/service/run"
	"audio-ui/internal/service/workflow"

	"go.uber.org/zap"
)

type runSetup struct {
	handler *handler.RunService
	manager *runservice.Manager
	audit   *audit.Logger
}

type runPermissionFactory struct {
	audit port.AuditSink
}

func (f *runPermissionFactory) NewRunPermissions(scope port.RunScope, resolver port.ScopedPathResolver, publish func(valueobject.AgentEvent)) (port.RunPermissionController, error) {
	if f.audit == nil {
		return nil, fmt.Errorf("AUDIT_UNAVAILABLE")
	}
	return permission.New(permission.Options{
		RunID: scope.RunID, TreeID: scope.TreeID, NodeID: scope.NodeID, ChainID: scope.ChainID,
		Paths: resolver, Audit: f.audit, Publish: publish,
	}), nil
}

func newRunSetup(layout valueobject.Layout, version string, logger *zap.Logger, bus *wailsEventBus, projectService *project.Service, workflowService *workflow.Service) (*runSetup, error) {
	auditLogger, auditErr := audit.New(layout.AuditFile)
	if auditErr != nil {
		logger.Error("audit logger init failed", zap.Error(auditErr))
	}
	permissionFactory := &runPermissionFactory{audit: auditLogger}

	resolveCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	native, nativeErr := claudecli.ResolveNative(resolveCtx, os.Getenv("AUDIOUI_CLAUDE_PATH"))
	cancel()
	var driver port.AgentDriver
	if nativeErr == nil {
		hostPath, execErr := os.Executable()
		if execErr != nil {
			hostPath = ""
		}
		driver = claudecli.NewDriver(native, hostPath, func(format string, args ...any) {
			logger.Info("agent", zap.String("detail", fmt.Sprintf(format, args...)))
		})
	} else {
		logger.Warn("claude native cli unavailable", zap.Error(nativeErr))
	}

	artifactStores := persistence.NewArtifactStoreProvider(layout)
	contextBuilder := contextservice.NewBuilder(artifactStores)
	toolServers := &toolServerFactory{root: layout.Root, version: version, runTools: workflowService}
	manager := runservice.New(runservice.Options{
		Projects: projectService, Workflow: workflowService,
		Contexts: contextBuilder, Workspaces: workspace.NewRunProvider(layout),
		RunStores: persistence.NewRunStoreProvider(layout), RunConfigs: paths.NewRunConfigStore(layout),
		ToolServers: toolServers, Permissions: permissionFactory, Driver: driver, Publisher: bus,
		Logf: func(format string, args ...any) {
			logger.Info("run", zap.String("detail", fmt.Sprintf(format, args...)))
		},
	})
	if err := manager.RecoverInterrupted(context.Background()); err != nil {
		if auditLogger != nil {
			_ = auditLogger.Close()
		}
		return nil, fmt.Errorf("恢复未完成运行失败: %w", err)
	}
	projectService.SetRunActiveCheck(manager.IsBusy)
	return &runSetup{handler: handler.NewRunService(manager), manager: manager, audit: auditLogger}, nil
}
