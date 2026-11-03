package app

import (
	"sync"

	"audio-ui/internal/domain/valueobject"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type wailsEventBus struct {
	mu          sync.Mutex
	app         *application.App
	pending     []valueobject.AgentEvent
	pendingRuns []valueobject.RunEvent
	started     bool
}

func newWailsEventBus() *wailsEventBus {
	return &wailsEventBus{}
}

func (b *wailsEventBus) SetApp(app *application.App) {
	b.mu.Lock()
	b.app = app
	b.mu.Unlock()
}

func (b *wailsEventBus) Start() {
	b.mu.Lock()
	b.started = true
	pending := b.pending
	pendingRuns := b.pendingRuns
	b.pending = nil
	b.pendingRuns = nil
	b.mu.Unlock()
	for _, event := range pending {
		b.emit(event)
	}
	for _, event := range pendingRuns {
		b.emitRun(event)
	}
}

func (b *wailsEventBus) Publish(event valueobject.AgentEvent) {
	b.mu.Lock()
	if b.app == nil || !b.started {
		b.pending = append(b.pending, event)
		if len(b.pending) > 1000 {
			b.pending = b.pending[len(b.pending)-1000:]
		}
		b.mu.Unlock()
		return
	}
	b.mu.Unlock()
	b.emit(event)
}

func (b *wailsEventBus) PublishRun(event valueobject.RunEvent) {
	b.mu.Lock()
	if b.app == nil || !b.started {
		b.pendingRuns = append(b.pendingRuns, event)
		if len(b.pendingRuns) > 1000 {
			b.pendingRuns = b.pendingRuns[len(b.pendingRuns)-1000:]
		}
		b.mu.Unlock()
		return
	}
	b.mu.Unlock()
	b.emitRun(event)
}

func (b *wailsEventBus) emit(event valueobject.AgentEvent) {
	b.mu.Lock()
	app := b.app
	b.mu.Unlock()
	if app == nil {
		return
	}
	name := "agent:" + string(event.Type)
	app.Event.Emit(name, event)
}

func (b *wailsEventBus) emitRun(event valueobject.RunEvent) {
	b.mu.Lock()
	app := b.app
	b.mu.Unlock()
	if app == nil {
		return
	}
	app.Event.Emit("run:"+string(event.Type), event)
}
