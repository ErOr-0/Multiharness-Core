package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"multiharness-core/internal/agent"
	"multiharness-core/internal/config"
	"multiharness-core/internal/testkit"
)

type mcpClient struct {
	t     *testing.T
	in    *io.PipeWriter
	lines chan map[string]any
	notes []map[string]any
	next  int
}

func startMCP(t *testing.T) *mcpClient {
	t.Helper()
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	c := &mcpClient{t: t, in: inW, lines: make(chan map[string]any, 64)}
	done := make(chan struct{})
	go func() {
		CmdMCP(nil, inR, outW, io.Discard)
		outW.Close()
		close(done)
	}()
	go func() {
		scanner := bufio.NewScanner(outR)
		scanner.Buffer(make([]byte, 64<<10), 8<<20)
		for scanner.Scan() {
			var msg map[string]any
			if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
				t.Errorf("the server wrote a line that is not JSON: %q", scanner.Text())
				continue
			}
			c.lines <- msg
		}
		close(c.lines)
	}()
	t.Cleanup(func() {
		inW.Close()
		<-done
	})
	return c
}

func (c *mcpClient) request(method string, params any) map[string]any {
	c.t.Helper()
	c.next++
	id := c.next
	data, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
	if _, err := c.in.Write(append(data, '\n')); err != nil {
		c.t.Fatal(err)
	}
	timeout := time.After(60 * time.Second)
	for {
		select {
		case msg, ok := <-c.lines:
			if !ok {
				c.t.Fatalf("the server stopped before answering %s", method)
			}
			if msg["id"] == nil {
				c.notes = append(c.notes, msg)
				continue
			}
			if msg["id"] == float64(id) {
				return msg
			}
		case <-timeout:
			c.t.Fatalf("no answer to %s", method)
		}
	}
}

func (c *mcpClient) tool(name string, args any, meta ...map[string]any) (map[string]any, bool) {
	c.t.Helper()
	params := map[string]any{"name": name, "arguments": args}
	if len(meta) > 0 {
		params["_meta"] = meta[0]
	}
	msg := c.request("tools/call", params)
	result, _ := msg["result"].(map[string]any)
	if result == nil {
		c.t.Fatalf("%s: no result in %v", name, msg)
	}
	text := result["content"].([]any)[0].(map[string]any)["text"].(string)
	var out map[string]any
	if result["isError"] == true {
		return map[string]any{"error": text}, true
	}
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		var list []any
		if json.Unmarshal([]byte(text), &list) == nil {
			return map[string]any{"list": list}, false
		}
		c.t.Fatalf("%s returned %q", name, text)
	}
	return out, false
}

func TestMCPHandshake(t *testing.T) {
	testkit.UseConfig(t, config.Config{Planner: "claude", Implementer: "codex", Router: "laya"})
	c := startMCP(t)
	init := c.request("initialize", map[string]any{"protocolVersion": "2025-06-18", "clientInfo": map[string]any{"name": "claude-code"}, "capabilities": map[string]any{}})
	result := init["result"].(map[string]any)
	instructions := result["instructions"].(string)
	if result["protocolVersion"] != "2025-06-18" || result["serverInfo"].(map[string]any)["name"] != "harness-bridge" {
		t.Fatalf("initialize: %v", result)
	}
	for _, want := range []string{"Codex CLI", "Call route", "Call delegate", "session_id", "network_access=true"} {
		if !strings.Contains(instructions, want) {
			t.Errorf("instructions lack %q:\n%s", want, instructions)
		}
	}
	if len(instructions) > mcpInstructionMax {
		t.Errorf("instructions are %d characters; Claude Code reads only %d", len(instructions), mcpInstructionMax)
	}
	var names []string
	for _, tool := range c.request("tools/list", map[string]any{})["result"].(map[string]any)["tools"].([]any) {
		names = append(names, tool.(map[string]any)["name"].(string))
	}
	if strings.Join(names, ",") != "delegate,wait,cancel,route,agents" {
		t.Fatalf("tools %v", names)
	}
	if pong := c.request("ping", nil); pong["result"] == nil {
		t.Fatalf("ping: %v", pong)
	}
	if missing := c.request("resources/list", nil); missing["error"].(map[string]any)["code"] != float64(-32601) {
		t.Fatalf("unknown method: %v", missing)
	}
}

func TestMCPNegotiatesAnUnknownVersionDown(t *testing.T) {
	s := &mcpServer{waitDefault: 50 * time.Second}
	got := s.initialize(json.RawMessage(`{"protocolVersion":"2099-01-01","clientInfo":{"name":"codex-mcp-client"}}`))
	if got["protocolVersion"] != mcpLatestVersion || s.waitDefault != 50*time.Second {
		t.Fatalf("%v, wait %s", got, s.waitDefault)
	}
	if !strings.Contains(got["instructions"].(string), "No implementer is set up yet") {
		t.Fatalf("instructions without setup: %s", got["instructions"])
	}
}

func TestMCPDelegateAndWait(t *testing.T) {
	testkit.FakeCLIs(t, "codex")
	testkit.UseConfig(t, config.Config{Planner: "claude", Implementer: "codex"})
	t.Setenv(testkit.FakeEnv, "codex-ok")
	dir := testkit.GitRepo(t)
	c := startMCP(t)
	c.request("initialize", map[string]any{"protocolVersion": "2025-06-18"})
	started, failed := c.tool("delegate", map[string]any{"task": "add hello", "directory": dir})
	if failed || started["status"] != agent.StatusRunning || started["harness"] != "codex" {
		t.Fatalf("delegate: %v", started)
	}
	if next := started["next"].(string); next != fmt.Sprintf("call wait with job %q", started["job"]) {
		t.Fatalf("next %q", next)
	}
	done, _ := c.tool("wait", map[string]any{"job": started["job"], "max_seconds": 30})
	if done["status"] != agent.StatusCompleted || done["session_id"] != "thread-123" || done["next"] != `to send fixes: call delegate with session_id "thread-123"` {
		t.Fatalf("wait: %v", done)
	}
	if status := done["git"].(map[string]any)["status"].([]any); len(status) != 1 || status[0] != "?? hello.txt" {
		t.Fatalf("git: %v", done["git"])
	}
	agents, _ := c.tool("agents", map[string]any{})
	for _, a := range agents["list"].([]any) {
		info := a.(map[string]any)
		if info["name"] == "codex" && (info["installed"] != true || info["role"] != "implementer") {
			t.Fatalf("agents: %v", info)
		}
	}
}

func TestMCPWaitReportsProgressAndCancel(t *testing.T) {
	testkit.FakeCLIs(t, "codex")
	t.Setenv(testkit.FakeEnv, "codex-slow")
	old := progressEvery
	progressEvery = 100 * time.Millisecond
	t.Cleanup(func() { progressEvery = old })
	c := startMCP(t)
	c.request("initialize", map[string]any{"protocolVersion": "2025-06-18"})
	started, _ := c.tool("delegate", map[string]any{"task": "x", "directory": t.TempDir(), "agent": "codex"})
	running, _ := c.tool("wait", map[string]any{"job": started["job"], "max_seconds": 2}, map[string]any{"progressToken": "tok-1"})
	if running["status"] != agent.StatusRunning {
		t.Fatalf("wait: %v", running)
	}
	var progress int
	for _, n := range c.notes {
		if n["method"] == "notifications/progress" && n["params"].(map[string]any)["progressToken"] == "tok-1" {
			progress++
		}
	}
	if progress == 0 {
		t.Fatalf("no progress notifications in %v", c.notes)
	}
	cancelled, _ := c.tool("cancel", map[string]any{"job": started["job"]})
	if cancelled["status"] != agent.StatusCancelled {
		t.Fatalf("cancel: %v", cancelled)
	}
}

func TestMCPRejectsBadCalls(t *testing.T) {
	c := startMCP(t)
	c.request("initialize", map[string]any{"protocolVersion": "2025-06-18"})
	for _, tc := range []struct {
		tool string
		args map[string]any
		want string
	}{
		{"delegate", map[string]any{"task": " ", "directory": t.TempDir(), "agent": "codex"}, "task is empty"},
		{"delegate", map[string]any{"task": "x", "directory": t.TempDir()}, "until setup has chosen an implementer"},
		{"delegate", map[string]any{"task": "x", "directory": t.TempDir(), "agent": "muse", "session_id": "s"}, "cannot continue a session"},
		{"wait", map[string]any{"job": "../x"}, "is not a job ID"},
		{"route", map[string]any{"request": "x"}, "--provider is required"},
		{"teleport", map[string]any{}, `unknown tool "teleport"`},
	} {
		out, failed := c.tool(tc.tool, tc.args)
		if !failed || !strings.Contains(out["error"].(string), tc.want) {
			t.Errorf("%s %v: got %v, want an error containing %q", tc.tool, tc.args, out, tc.want)
		}
	}
}

func TestMCPRouteUsesTheSavedRouter(t *testing.T) {
	srv, _ := testkit.DecisionServer(t, 200, `{"answers":{"task_routing":{"choice":"answer","confidence":0.97}}}`)
	t.Setenv("LAYA_ENDPOINT", srv.URL)
	testkit.UseConfig(t, config.Config{Implementer: "codex", Router: "laya"})
	c := startMCP(t)
	c.request("initialize", map[string]any{"protocolVersion": "2025-06-18"})
	d, failed := c.tool("route", map[string]any{"request": "what does this function do?"})
	if failed || d["route"] != "answer" || d["provider"] != "laya" {
		t.Fatalf("route: %v", d)
	}
}
