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
	"audio-ui/internal/service/permission"
	"audio-ui/internal/service/session"

	"go.uber.org/zap"
)

type agentSetup struct {
	handler    *handler.AgentService
	session    *session.Service
	permission *permission.Service
	audit      *audit.Logger
}

func newAgentSetup(layout valueobject.Layout, version string, logger *zap.Logger, bus *wailsEventBus, runTools port.RunToolService) (*agentSetup, error) {
	guard, err := paths.NewGuard(layout.Root, layout.WorkspaceDir)
	if err != nil {
		return nil, err
	}
	auditLogger, auditErr := audit.New(layout.AuditFile)
	cliErr := ""
	cliAvailable := true
	if auditErr != nil {
		cliAvailable = false
		cliErr = "审计日志不可用: " + auditErr.Error()
		logger.Error("audit logger init failed", zap.Error(auditErr))
	}

	resolveCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	native, nativeErr := claudecli.ResolveNative(resolveCtx, os.Getenv("AUDIOUI_CLAUDE_PATH"))
	cancel()
	if nativeErr != nil {
		cliAvailable = false
		cliErr = nativeErr.Error()
		logger.Warn("claude native cli unavailable", zap.Error(nativeErr))
	}

	var publishAgentEvent func(valueobject.AgentEvent)
	permissions := permission.New(permission.Options{
		ChainID: valueobject.DefaultChainID,
		Paths:   guard,
		Audit:   auditLogger,
		Publish: func(event valueobject.AgentEvent) {
			if publishAgentEvent != nil {
				publishAgentEvent(event)
			}
		},
	})
	store := persistence.NewSessionStore(layout.SessionsFile)
	tools := &toolServerFactory{root: layout.Root, workspace: layout.WorkspaceDir, version: version, runTools: runTools}

	var driver port.AgentDriver
	if nativeErr == nil {
		hostPath, execErr := os.Executable()
		if execErr != nil {
			hostPath = ""
		}
		driver = claudecli.NewDriver(native, hostPath, func(format string, args ...any) {
			logger.Info("agent", zap.String("detail", fmt.Sprintf(format, args...)))
		})
	}

	sessionService := session.New(session.Options{
		Layout:       layout,
		Driver:       driver,
		Store:        store,
		Publisher:    bus,
		Tools:        tools,
		Permissions:  permissions,
		Questions:    permissions,
		CLIAvailable: cliAvailable,
		CLIError:     cliErr,
		Logf: func(format string, args ...any) {
			logger.Info("session", zap.String("detail", fmt.Sprintf(format, args...)))
		},
	})
	publishAgentEvent = sessionService.HandleEvent
	permissions.SetResolvedCallback(func(kind, id string) {
		if kind == "permission" {
			sessionService.ClearPermission(id)
		} else {
			sessionService.ClearQuestion(id)
		}
	})
	return &agentSetup{
		handler:    handler.NewAgentService(sessionService),
		session:    sessionService,
		permission: permissions,
		audit:      auditLogger,
	}, nil
}
