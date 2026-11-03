package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"audio-ui/internal/domain/port"
	"audio-ui/internal/domain/valueobject"
)

var (
	ErrBusy             = errors.New("BUSY: current turn is still running")
	ErrAgentUnavailable = errors.New("AGENT_UNAVAILABLE")
	ErrSessionClosed    = errors.New("session closed")
	errStaleGeneration  = errors.New("stale session generation")
)

type Options struct {
	Layout       valueobject.Layout
	Driver       port.AgentDriver
	Store        port.SessionStore
	Publisher    port.EventPublisher
	Tools        port.ToolServerFactory
	Permissions  port.AgentPermissionController
	Questions    port.AgentQuestionController
	CLIAvailable bool
	CLIError     string
	Logf         func(format string, args ...any)
}

type Service struct {
	opts Options

	opMu           sync.Mutex
	persistMu      sync.Mutex
	mu             sync.Mutex
	messageCounter atomic.Uint64
	state          valueobject.AgentState
	record         valueobject.SessionRecord
	driver         port.AgentSession
	tool           port.ToolServer
	closed         bool
	seq            uint64
	generation     uint64
	resumeCancel   context.CancelFunc
	mcpPath        string
}

func New(opts Options) *Service {
	if opts.Logf == nil {
		opts.Logf = func(string, ...any) {}
	}
	status := valueobject.SessionIdle
	if !opts.CLIAvailable {
		status = valueobject.SessionUnavailable
	}
	return &Service{
		opts: opts,
		state: valueobject.AgentState{
			Version:      1,
			ChainID:      valueobject.DefaultChainID,
			Adapter:      "claude",
			Status:       status,
			CLIAvailable: opts.CLIAvailable,
			CLIError:     opts.CLIError,
			Transcript:   []valueobject.TranscriptItem{},
			Permissions:  []valueobject.PermissionRequest{},
			Questions:    []valueobject.QuestionRequest{},
			Notices:      []string{},
		},
		record: valueobject.SessionRecord{Adapter: "claude", Status: status},
	}
}

// Start loads persisted metadata and starts resume in the background.
func (s *Service) Start(ctx context.Context) {
	file, err := s.opts.Store.Load(ctx)
	if err != nil {
		s.mu.Lock()
		s.state.Status = valueobject.SessionResumeFailed
		s.state.ResumeError = err.Error()
		s.state.Notices = append(s.state.Notices, "会话元数据损坏，未覆盖原文件")
		s.mu.Unlock()
		s.publishStatus()
		return
	}
	record, ok := file.Chains[valueobject.DefaultChainID]
	if !ok || len(record.ResumeRef) == 0 {
		s.publishStatus()
		return
	}
	s.mu.Lock()
	s.record = record
	s.state.SessionID = displaySessionID(record.ResumeRef)
	s.state.DisplayID = record.DisplayID
	s.state.Transcript = append([]valueobject.TranscriptItem{}, record.Transcript...)
	s.state.Status = valueobject.SessionResuming
	s.mu.Unlock()
	s.publishStatus()

	go func() {
		startCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		s.mu.Lock()
		s.resumeCancel = cancel
		generation := s.generation
		s.mu.Unlock()
		defer func() {
			cancel()
			s.mu.Lock()
			s.resumeCancel = nil
			s.mu.Unlock()
		}()
		if err := s.startDriver(startCtx, record.ResumeRef, generation); err != nil {
			if errors.Is(err, errStaleGeneration) {
				return
			}
			s.mu.Lock()
			s.state.Status = valueobject.SessionResumeFailed
			s.state.ResumeError = err.Error()
			s.state.Notices = append(s.state.Notices, "会话恢复失败，请选择新开会话或取消")
			s.mu.Unlock()
			s.persist()
			s.publishStatus()
			return
		}
		s.mu.Lock()
		if s.state.Status == valueobject.SessionResuming {
			s.state.Status = valueobject.SessionIdle
		}
		s.mu.Unlock()
		s.persist()
		s.publishStatus()
	}()
}

func (s *Service) HandleEvent(event valueobject.AgentEvent) {
	s.handleEvent(event)
}

func (s *Service) State() valueobject.AgentState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snapshotLocked()
}

func (s *Service) Send(ctx context.Context, text string) error {
	if s.opts.Driver == nil {
		return ErrAgentUnavailable
	}
	s.opMu.Lock()
	defer s.opMu.Unlock()

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return ErrSessionClosed
	}
	if !s.state.CLIAvailable {
		err := ErrAgentUnavailable
		if s.state.CLIError != "" {
			err = fmt.Errorf("%w: %s", ErrAgentUnavailable, s.state.CLIError)
		}
		s.mu.Unlock()
		return err
	}
	if s.state.Status == valueobject.SessionRunning || s.state.Status == valueobject.SessionResuming || s.state.Status == valueobject.SessionStarting {
		s.mu.Unlock()
		return ErrBusy
	}
	if s.state.Status == valueobject.SessionResumeFailed {
		s.mu.Unlock()
		return errors.New("SESSION_RESUME_FAILED: choose new session or cancel")
	}
	needStart := s.driver == nil
	generation := s.generation
	s.mu.Unlock()

	if needStart {
		if err := s.startDriver(ctx, nil, generation); err != nil {
			s.mu.Lock()
			s.state.Status = valueobject.SessionError
			s.state.Notices = append(s.state.Notices, "Agent 启动失败: "+err.Error())
			s.mu.Unlock()
			s.persist()
			s.publishStatus()
			return err
		}
	}

	s.mu.Lock()
	item := valueobject.TranscriptItem{
		ID:        fmt.Sprintf("user-%d", s.messageCounter.Add(1)),
		Kind:      valueobject.TranscriptMessage,
		Role:      "user",
		Text:      text,
		CreatedAt: time.Now().Format(time.RFC3339),
	}
	s.appendTranscriptLocked(item)
	s.state.Status = valueobject.SessionRunning
	s.mu.Unlock()
	s.persist()
	s.publishStatus()

	s.mu.Lock()
	driver := s.driver
	s.mu.Unlock()
	if driver == nil {
		return errors.New("AGENT_UNAVAILABLE: session not started")
	}
	if err := driver.Send(ctx, text); err != nil {
		s.handleEvent(valueobject.AgentEvent{Version: 1, Type: valueobject.EventError, Error: &valueobject.AgentError{
			Code: "SEND_FAILED", Message: err.Error(), Recoverable: true,
		}})
		return err
	}
	return nil
}

func (s *Service) ClearPermission(id string) {
	s.mu.Lock()
	s.removePermissionLocked(id)
	s.mu.Unlock()
	s.persist()
	s.publishStatus()
}

func (s *Service) ClearQuestion(id string) {
	s.mu.Lock()
	s.removeQuestionLocked(id)
	s.mu.Unlock()
	s.persist()
	s.publishStatus()
}

func (s *Service) RespondPermission(id string, decision valueobject.PermissionDecision) error {
	if s.opts.Permissions == nil {
		return errors.New("permission controller unavailable")
	}
	if err := s.opts.Permissions.ResolvePermission(id, decision); err != nil {
		return err
	}
	s.mu.Lock()
	s.removePermissionLocked(id)
	s.mu.Unlock()
	s.persist()
	s.publishStatus()
	return nil
}

func (s *Service) RespondQuestion(id string, answer valueobject.QuestionAnswer) error {
	if s.opts.Questions == nil {
		return errors.New("question controller unavailable")
	}
	if err := s.opts.Questions.ResolveQuestion(id, answer); err != nil {
		return err
	}
	s.mu.Lock()
	s.removeQuestionLocked(id)
	s.mu.Unlock()
	s.persist()
	s.publishStatus()
	return nil
}

func (s *Service) NewSession(ctx context.Context) error {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return ErrSessionClosed
	}
	s.generation++
	cancelResume := s.resumeCancel
	s.resumeCancel = nil
	s.mu.Unlock()
	if cancelResume != nil {
		cancelResume()
	}
	s.closeRuntime()
	s.mu.Lock()
	s.record = valueobject.SessionRecord{Adapter: "claude", Status: valueobject.SessionIdle}
	s.state.SessionID = ""
	s.state.DisplayID = "新会话"
	s.state.Transcript = []valueobject.TranscriptItem{}
	s.state.Permissions = []valueobject.PermissionRequest{}
	s.state.Questions = []valueobject.QuestionRequest{}
	s.state.Notices = []string{}
	s.state.Status = valueobject.SessionIdle
	s.state.ResumeError = ""
	s.mu.Unlock()
	if setter, ok := s.opts.Permissions.(interface{ SetSessionID(string) }); ok {
		setter.SetSessionID("")
	}
	s.persist()
	s.publishStatus()
	generation := s.generation
	if err := s.startDriver(ctx, nil, generation); err != nil {
		s.mu.Lock()
		s.state.Status = valueobject.SessionError
		s.state.Notices = append(s.state.Notices, "新会话启动失败: "+err.Error())
		s.mu.Unlock()
		s.persist()
		s.publishStatus()
		return err
	}
	s.mu.Lock()
	s.state.Status = valueobject.SessionIdle
	s.mu.Unlock()
	s.publishStatus()
	return nil
}

func (s *Service) CancelResume() {
	s.mu.Lock()
	s.generation++
	cancelResume := s.resumeCancel
	s.resumeCancel = nil
	if s.state.Status == valueobject.SessionResumeFailed {
		s.state.Status = valueobject.SessionIdle
		s.state.ResumeError = ""
		s.state.Notices = []string{}
		s.state.DisplayID = "新会话"
		s.state.SessionID = ""
		s.state.Transcript = []valueobject.TranscriptItem{}
		s.record.ResumeRef = nil
	}
	s.mu.Unlock()
	if cancelResume != nil {
		cancelResume()
	}
	s.persist()
	s.publishStatus()
}

func (s *Service) Interrupt(ctx context.Context) error {
	s.mu.Lock()
	driver := s.driver
	s.mu.Unlock()
	if driver == nil {
		return nil
	}
	return driver.Interrupt(ctx)
}

func (s *Service) Close() {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	s.mu.Lock()
	s.generation++
	cancelResume := s.resumeCancel
	s.resumeCancel = nil
	s.closed = true
	s.mu.Unlock()
	if cancelResume != nil {
		cancelResume()
	}
	s.closeRuntime()
}

func (s *Service) closeRuntime() {
	s.mu.Lock()
	driver := s.driver
	tool := s.tool
	s.driver = nil
	s.tool = nil
	mcpPath := s.mcpPath
	s.mcpPath = ""
	s.mu.Unlock()
	if driver != nil {
		_ = driver.Close()
	}
	if tool != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		_ = tool.Stop(ctx)
		cancel()
	}
	if mcpPath != "" {
		_ = os.Remove(mcpPath)
	}
}

func (s *Service) startDriver(ctx context.Context, ref json.RawMessage, generation uint64) error {
	s.mu.Lock()
	if s.closed || s.generation != generation {
		s.mu.Unlock()
		return errStaleGeneration
	}
	if s.driver != nil {
		s.mu.Unlock()
		return nil
	}
	if !s.state.CLIAvailable {
		s.mu.Unlock()
		return ErrAgentUnavailable
	}
	displayID := s.state.DisplayID
	s.mu.Unlock()

	if s.opts.Tools == nil {
		return errors.New("tool server unavailable")
	}
	tool, err := s.opts.Tools.New(valueobject.DefaultChainID, displayID, s.opts.Questions, s.onReply)
	if err != nil {
		return err
	}
	if err := tool.Start(ctx); err != nil {
		return err
	}
	mcpPath := filepath.Join(s.opts.Layout.AgentDir, "mcp.json")
	if err := tool.WriteConfig(mcpPath); err != nil {
		_ = tool.Stop(context.Background())
		return err
	}

	spec := port.DriverSpec{
		ChainID:      valueobject.DefaultChainID,
		Workspace:    s.opts.Layout.WorkspaceDir,
		SystemPrompt: "You are AudioUI's local content agent. Work only inside the provided workspace. Use ask_user when a decision is needed. Use reply(kind=message) for chat output; graph tools are not available in M1.",
		BuiltinTools: []string{"Read", "Glob", "Grep", "Write", "Edit", "Bash"},
		AllowedTools: []string{
			"mcp__audioui__get_chain", "mcp__audioui__reply", "mcp__audioui__read_file",
			"mcp__audioui__list_dir", "mcp__audioui__grep", "mcp__audioui__stat", "mcp__audioui__ask_user",
		},
		MCPConfigPath: mcpPath,
		MCPURL:        tool.URL(),
		MCPToken:      tool.Token(),
		EventSink: func(event valueobject.AgentEvent) {
			if !s.isStale(generation) {
				s.handleEvent(event)
			}
		},
		OnResumeRef: func(ref json.RawMessage) {
			if !s.isStale(generation) {
				s.onResumeRef(ref)
			}
		},
		PermissionGate: s.opts.Permissions,
		QuestionGate:   s.opts.Questions,
		ToolPolicy:     s.opts.Permissions,
	}
	var driver port.AgentSession
	if len(ref) > 0 {
		driver, err = s.opts.Driver.Resume(ctx, spec, ref)
	} else {
		driver, err = s.opts.Driver.Start(ctx, spec)
	}
	if err != nil {
		_ = tool.Stop(context.Background())
		_ = os.Remove(mcpPath)
		return err
	}

	s.mu.Lock()
	if s.closed || s.generation != generation {
		s.mu.Unlock()
		_ = driver.Close()
		_ = tool.Stop(context.Background())
		_ = os.Remove(mcpPath)
		return errStaleGeneration
	}
	s.driver = driver
	s.tool = tool
	s.mcpPath = mcpPath
	s.state.Adapter = "claude"
	if s.state.Status == valueobject.SessionUnavailable {
		s.state.Status = valueobject.SessionIdle
	}
	s.mu.Unlock()
	return nil
}

func (s *Service) isStale(generation uint64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed || s.generation != generation
}

func (s *Service) onReply(text string) {
	if text == "" {
		return
	}
	s.handleEvent(valueobject.AgentEvent{Version: 1, Type: valueobject.EventMessage, Message: &valueobject.TranscriptItem{
		ID:        fmt.Sprintf("reply-%d", s.messageCounter.Add(1)),
		Kind:      valueobject.TranscriptMessage,
		Role:      "assistant",
		Text:      text,
		CreatedAt: time.Now().Format(time.RFC3339),
	}})
}

func (s *Service) onResumeRef(ref json.RawMessage) {
	var payload struct {
		SessionID string `json:"sessionId"`
	}
	if len(ref) == 0 || json.Unmarshal(ref, &payload) != nil {
		return
	}
	s.mu.Lock()
	s.record.Adapter = "claude"
	s.record.ResumeRef = append(json.RawMessage{}, ref...)
	s.record.DisplayID = displayLabel(payload.SessionID)
	s.record.Workspace = s.opts.Layout.WorkspaceDir
	s.state.SessionID = payload.SessionID
	s.state.DisplayID = s.record.DisplayID
	s.mu.Unlock()
	if setter, ok := s.opts.Permissions.(interface{ SetSessionID(string) }); ok {
		setter.SetSessionID(payload.SessionID)
	}
	s.persist()
	s.publishStatus()
}

func (s *Service) handleEvent(event valueobject.AgentEvent) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.seq++
	event.Version = 1
	event.Seq = s.seq
	switch event.Type {
	case valueobject.EventStatus:
		if event.State != nil && event.State.Status != "" {
			s.state.Status = event.State.Status
		}
		event.State = ptrState(s.snapshotLocked())
	case valueobject.EventMessage:
		if event.Message != nil {
			s.applyMessageLocked(*event.Message)
		}
	case valueobject.EventTool:
		if event.Tool != nil {
			s.applyToolLocked(*event.Tool)
		}
	case valueobject.EventPermission:
		if event.Permission != nil {
			s.addPermissionLocked(*event.Permission)
		}
	case valueobject.EventQuestion:
		if event.Question != nil {
			s.addQuestionLocked(*event.Question)
		}
	case valueobject.EventError:
		if event.Error != nil {
			s.state.Notices = append(s.state.Notices, event.Error.Message)
			if s.state.Status == valueobject.SessionRunning {
				s.state.Status = valueobject.SessionError
			}
		}
	}
	s.syncRecordLocked()
	s.mu.Unlock()

	if event.Type != valueobject.EventMessage || event.Message == nil || !event.Message.Streaming {
		s.persist()
	}
	if s.opts.Publisher != nil {
		s.opts.Publisher.Publish(event)
	}
}

func (s *Service) applyMessageLocked(item valueobject.TranscriptItem) {
	for i := range s.state.Transcript {
		if s.state.Transcript[i].ID != item.ID {
			continue
		}
		if item.Streaming && s.state.Transcript[i].Streaming {
			s.state.Transcript[i].Text += item.Text
		} else {
			s.state.Transcript[i] = item
		}
		s.trimTranscriptLocked()
		return
	}
	s.state.Transcript = append(s.state.Transcript, item)
	s.trimTranscriptLocked()
}

func (s *Service) applyToolLocked(item valueobject.TranscriptItem) {
	for i := range s.state.Transcript {
		if s.state.Transcript[i].ID == item.ID && s.state.Transcript[i].Kind == valueobject.TranscriptTool {
			if item.ToolName != "" {
				s.state.Transcript[i].ToolName = item.ToolName
			}
			if item.ToolStatus != "" {
				s.state.Transcript[i].ToolStatus = item.ToolStatus
			}
			if item.ToolSummary != "" {
				s.state.Transcript[i].ToolSummary = item.ToolSummary
			}
			return
		}
	}
	s.state.Transcript = append(s.state.Transcript, item)
	s.trimTranscriptLocked()
}

func (s *Service) trimTranscriptLocked() {
	if len(s.state.Transcript) > valueobject.MaxTranscriptItems {
		s.state.Transcript = s.state.Transcript[len(s.state.Transcript)-valueobject.MaxTranscriptItems:]
	}
}

func (s *Service) addPermissionLocked(request valueobject.PermissionRequest) {
	for _, item := range s.state.Permissions {
		if item.ID == request.ID {
			return
		}
	}
	s.state.Permissions = append(s.state.Permissions, request)
	if s.state.Status != valueobject.SessionUnavailable && s.state.Status != valueobject.SessionResumeFailed {
		s.state.Status = valueobject.SessionRunning
	}
}

func (s *Service) addQuestionLocked(request valueobject.QuestionRequest) {
	for _, item := range s.state.Questions {
		if item.ID == request.ID {
			return
		}
	}
	s.state.Questions = append(s.state.Questions, request)
	if s.state.Status != valueobject.SessionUnavailable && s.state.Status != valueobject.SessionResumeFailed {
		s.state.Status = valueobject.SessionRunning
	}
}

func (s *Service) removePermissionLocked(id string) {
	filtered := s.state.Permissions[:0]
	for _, item := range s.state.Permissions {
		if item.ID != id {
			filtered = append(filtered, item)
		}
	}
	s.state.Permissions = filtered
}

func (s *Service) removeQuestionLocked(id string) {
	filtered := s.state.Questions[:0]
	for _, item := range s.state.Questions {
		if item.ID != id {
			filtered = append(filtered, item)
		}
	}
	s.state.Questions = filtered
}

func (s *Service) appendTranscriptLocked(item valueobject.TranscriptItem) {
	s.state.Transcript = append(s.state.Transcript, item)
	s.trimTranscriptLocked()
}

func (s *Service) syncRecordLocked() {
	s.record.Status = s.state.Status
	s.record.Workspace = s.opts.Layout.WorkspaceDir
	s.record.DisplayID = s.state.DisplayID
	s.record.Transcript = append([]valueobject.TranscriptItem{}, s.state.Transcript...)
	s.record.UpdatedAt = time.Now().Format(time.RFC3339)
}

func (s *Service) persist() {
	s.persistMu.Lock()
	defer s.persistMu.Unlock()
	s.mu.Lock()
	s.syncRecordLocked()
	file := valueobject.SessionsFile{
		Version:       valueobject.SessionFileVersion,
		ActiveChainID: valueobject.DefaultChainID,
		Chains:        map[string]valueobject.SessionRecord{valueobject.DefaultChainID: s.record},
	}
	s.mu.Unlock()
	if err := s.opts.Store.Save(context.Background(), file); err != nil {
		s.opts.Logf("session persist failed: %v", err)
	}
}

func (s *Service) publishStatus() {
	s.mu.Lock()
	s.seq++
	state := s.snapshotLocked()
	state.Seq = s.seq
	event := valueobject.AgentEvent{Version: 1, Type: valueobject.EventStatus, State: ptrState(state)}
	event.Seq = s.seq
	s.mu.Unlock()
	if s.opts.Publisher != nil {
		s.opts.Publisher.Publish(event)
	}
}

func (s *Service) snapshotLocked() valueobject.AgentState {
	copyState := s.state
	copyState.Transcript = append([]valueobject.TranscriptItem{}, s.state.Transcript...)
	copyState.Permissions = append([]valueobject.PermissionRequest{}, s.state.Permissions...)
	copyState.Questions = append([]valueobject.QuestionRequest{}, s.state.Questions...)
	copyState.Notices = append([]string{}, s.state.Notices...)
	copyState.Seq = s.seq
	return copyState
}

func ptrState(state valueobject.AgentState) *valueobject.AgentState { return &state }

func displayLabel(sessionID string) string {
	if len(sessionID) <= 8 {
		return sessionID
	}
	return sessionID[len(sessionID)-8:]
}

func displaySessionID(ref json.RawMessage) string {
	var payload struct {
		SessionID string `json:"sessionId"`
	}
	if json.Unmarshal(ref, &payload) == nil {
		return payload.SessionID
	}
	return ""
}
