package musecli

import (
	"encoding/json"
	"strings"
	"testing"
)

func event(kind, command, terminal, text string) string {
	b, _ := json.Marshal(map[string]any{"schema_version": 1, "payload_schema_version": 1, "payload_type": kind, "stream": map[string]string{"kind": "session", "id": "session"}, "payload": map[string]string{"command_id": command, "terminal": terminal, "text": text}})
	return string(b) + "\n"
}

func TestStreamRequiresCompleteMatchingRoot(t *testing.T) {
	start := event("run.lifecycle.started", "run", "", "")
	done := event("run.terminal.completed", "run", "completed", `{"summary":"done"}`)
	for _, data := range []string{done, start, start + done + done, start + event("run.terminal.completed", "other", "completed", "false success"), start + event("run.terminal.failed", "run", "failed", ""), start + strings.Replace(done, `"schema_version":1`, `"schema_version":2`, 1), start + `{"broken":`, start + strings.Repeat("x", MaxLine+1)} {
		s := &Stream{}
		_, _ = s.Write([]byte(data))
		if _, err := s.Finish(); err == nil {
			t.Fatal("accepted malformed/incomplete stream")
		}
	}
	s := &Stream{}
	for _, b := range []byte(start + done) {
		_, _ = s.Write([]byte{b})
	}
	text, err := s.Finish()
	if err != nil || text != `{"summary":"done"}` {
		t.Fatal(text, err)
	}
}

func TestStreamDoesNotAccumulateTranscript(t *testing.T) {
	s := &Stream{}
	_, _ = s.Write([]byte(event("run.lifecycle.started", "run", "", "")))
	for range 10000 {
		_, _ = s.Write([]byte(event("task.lifecycle.completed", "run", "", strings.Repeat("x", 1024))))
	}
	_, _ = s.Write([]byte(event("run.terminal.completed", "run", "completed", "done")))
	if text, err := s.Finish(); text != "done" || err != nil {
		t.Fatal(text, err)
	}
}
