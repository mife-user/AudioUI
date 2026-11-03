package run

import (
	"errors"
	"sync"
)

var (
	ErrBusy          = errors.New("BUSY")
	ErrGateOwner     = errors.New("RUN_GATE_OWNER_MISMATCH")
	ErrManagerClosed = errors.New("RUN_MANAGER_CLOSED")
)

// TurnGate enforces M2's single in-flight run without queueing.
type TurnGate struct {
	mu    sync.Mutex
	owner string
}

func NewTurnGate() *TurnGate { return &TurnGate{} }

func (g *TurnGate) Acquire(runID string) error {
	if runID == "" {
		return errors.New("runId is required")
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.owner != "" {
		return ErrBusy
	}
	g.owner = runID
	return nil
}

func (g *TurnGate) Release(runID string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.owner == "" {
		return nil
	}
	if g.owner != runID {
		return ErrGateOwner
	}
	g.owner = ""
	return nil
}

func (g *TurnGate) Owner() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.owner
}

func (g *TurnGate) Busy() bool { return g.Owner() != "" }
