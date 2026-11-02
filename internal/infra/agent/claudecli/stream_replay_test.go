package claudecli

import (
	"encoding/json"
	"os"
	"testing"

	"audio-ui/internal/domain/port"
	"audio-ui/internal/domain/valueobject"

	claudecode "github.com/severity1/claude-agent-sdk-go"
)

type fixtureStream struct {
	Type      string         `json:"type"`
	UUID      string         `json:"uuid"`
	SessionID string         `json:"session_id"`
	Event     map[string]any `json:"event"`
}

func TestStreamEventReplay(t *testing.T) {
	data, err := os.ReadFile("testdata/stream_events.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []fixtureStream
	if err := json.Unmarshal(data, &fixtures); err != nil {
		t.Fatal(err)
	}
	var events []valueobject.AgentEvent
	session := &agentSession{spec: port.DriverSpec{EventSink: func(event valueobject.AgentEvent) {
		events = append(events, event)
	}}}
	for _, fixture := range fixtures {
		event := &claudecode.StreamEvent{UUID: fixture.UUID, SessionID: fixture.SessionID, Event: fixture.Event}
		session.handleStreamEvent(event)
	}
	if len(events) != 2 {
		t.Fatalf("events = %d, want 2", len(events))
	}
	if events[0].Message == nil || events[1].Message == nil {
		t.Fatalf("message event missing: %+v", events)
	}
	if events[0].Message.Text+" "+events[1].Message.Text != "Hello  world" {
		t.Fatalf("stream text = %q + %q", events[0].Message.Text, events[1].Message.Text)
	}
	if events[0].Message.ID != events[1].Message.ID {
		t.Fatal("stream deltas should share an ID")
	}
}
