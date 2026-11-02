package app

import (
	"audio-ui/internal/domain/port"
	"audio-ui/internal/infra/agent/mcp"
)

type toolServerFactory struct {
	root      string
	workspace string
	version   string
	runTools  port.RunToolService
}

func (f *toolServerFactory) New(chainID, displayID string, questionGate port.QuestionGate, onReply func(string)) (port.ToolServer, error) {
	return f.newServer(chainID, displayID, questionGate, onReply, nil)
}

func (f *toolServerFactory) NewForRun(scope port.RunScope, workspaceRoot, displayID string, questionGate port.QuestionGate, onReply func(string), reader port.WorkspaceReader, lifecycle port.RunLifecycle) (port.ToolServer, error) {
	return mcp.New(mcp.Options{
		Root:            f.root,
		Workspace:       workspaceRoot,
		ChainID:         scope.ChainID,
		DisplayID:       displayID,
		Version:         f.version,
		QuestionGate:    questionGate,
		OnReply:         onReply,
		WorkspaceReader: reader,
		RunScope:        &scope,
		RunTools:        f.runTools,
		RunLifecycle:    lifecycle,
	})
}

func (f *toolServerFactory) newServer(chainID, displayID string, questionGate port.QuestionGate, onReply func(string), reader port.WorkspaceReader) (port.ToolServer, error) {
	return mcp.New(mcp.Options{
		Root:            f.root,
		Workspace:       f.workspace,
		ChainID:         chainID,
		DisplayID:       displayID,
		Version:         f.version,
		QuestionGate:    questionGate,
		OnReply:         onReply,
		WorkspaceReader: reader,
	})
}
