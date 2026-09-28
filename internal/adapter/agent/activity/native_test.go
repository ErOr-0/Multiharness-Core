package activity

import (
	"strings"
	"testing"
)

func TestNativeProgressDoesNotExposeApprovalOrReasoning(t *testing.T) {
	for _, data := range []string{`{"method":"item/fileChange/requestApproval","params":{"reason":"private approval"}}`, `{"method":"item/completed","params":{"item":{"type":"reasoning","text":"private reasoning"}}}`} {
		if event, ok := nativeActivity(Codex, []byte(data)); ok {
			t.Fatal(event)
		}
	}
	e, ok := nativeActivity(Codex, []byte(`{"method":"item/completed","params":{"item":{"type":"agentMessage","text":"Updated files"}}}`))
	if !ok || e.Kind != ResponseReceived || !strings.Contains(e.Text, "Updated files") {
		t.Fatal(e, ok)
	}
}
