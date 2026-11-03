package app

import (
	"audio-ui/internal/domain/valueobject"
	"audio-ui/internal/handler"
	"audio-ui/internal/service/session"
)

// agentSetup keeps the M1 binding surface available until M2-08 replaces the
// temporary UI. It is deliberately disabled: only RunService may launch an
// Agent in the M2 runtime.
type agentSetup struct {
	handler *handler.AgentService
	session *session.Service
}

func newAgentSetup(layout valueobject.Layout) *agentSetup {
	legacy := session.New(session.Options{
		Layout:       layout,
		CLIAvailable: false,
		CLIError:     "M1 会话已由 M2 一次性运行取代",
	})
	return &agentSetup{handler: handler.NewAgentService(legacy), session: legacy}
}
