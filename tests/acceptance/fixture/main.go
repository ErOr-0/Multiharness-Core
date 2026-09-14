// This executable is a protocol fixture, never a substitute for live AI proof.
// It records actual argv/stdin/cwd and edits files so acceptance steps can
// independently observe the application/process boundary.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

func main() {
	args := os.Args[1:]
	task, err := io.ReadAll(os.Stdin)
	must(err)
	cwd, err := os.Getwd()
	must(err)
	f, err := os.OpenFile(os.Getenv("BDD_RECORD"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	must(err)
	must(json.NewEncoder(f).Encode(map[string]any{"args": args, "task": string(task), "cwd": cwd, "pid": os.Getpid()}))
	must(f.Close())
	must(os.WriteFile("provider-edit.txt", []byte("native edit"), 0600))
	mode := os.Getenv("BDD_BEHAVIOR")
	session := "session_fixture_123"
	if mode == "different-session" {
		session = "session_replaced_456"
	}
	if mode == "no-session" {
		session = ""
	}
	provider := os.Getenv("BDD_PROVIDER")
	emit := func(value any) { must(json.NewEncoder(os.Stdout).Encode(value)) }
	switch provider {
	case "codex":
		emit(map[string]any{"type": "thread.started", "thread_id": session})
		emit(map[string]any{"type": "item.completed", "item": map[string]string{"type": "agent_message", "text": "Native response"}})
	case "opencode":
		emit(map[string]any{"type": "step_start", "sessionID": session})
		emit(map[string]any{"type": "text", "sessionID": session, "part": map[string]string{"text": "Native response"}})
	case "claude":
		emit(map[string]any{"type": "system", "session_id": session})
		emit(map[string]any{"type": "assistant", "message": map[string]any{"content": []any{map[string]string{"type": "text", "text": "Native response"}}}})
	default:
		panic("missing BDD_PROVIDER")
	}
	switch mode {
	case "hang":
		for {
			must(os.WriteFile("heartbeat.txt", []byte(time.Now().Format(time.RFC3339Nano)), 0600))
			time.Sleep(50 * time.Millisecond)
		}
	case "truncated":
		return
	case "malformed":
		fmt.Println("{invalid-json")
		return
	case "nonzero":
		fmt.Fprintln(os.Stderr, "provider fixture failed")
		os.Exit(7)
	}
	switch provider {
	case "codex":
		emit(map[string]string{"type": "turn.completed"})
	case "opencode":
		emit(map[string]any{"type": "step_finish", "sessionID": session, "part": map[string]string{"reason": "stop"}})
	case "claude":
		denials := []any{}
		if strings.EqualFold(mode, "denied") {
			denials = append(denials, map[string]string{"tool_name": "Bash"})
		}
		emit(map[string]any{"type": "result", "subtype": "success", "session_id": session, "result": "Native response", "permission_denials": denials})
	}
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
