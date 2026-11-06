package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

// harness-bridge mcp is a stdio MCP server (newline-delimited JSON-RPC 2.0),
// so a planner agent gets delegate, wait, cancel, route and agents tools
// without per-project files. The planner starts it; nothing else runs.

const (
	mcpServerName     = "harness-bridge"
	mcpLatestVersion  = "2025-11-25"
	mcpInstructionMax = 4096 // Claude Code cuts server instructions here

)

// progressEvery is how often a long wait reports progress.
var progressEvery = 20 * time.Second

var mcpVersions = []string{mcpLatestVersion, "2025-06-18", "2025-03-26", "2024-11-05"}

// version is set at release build time with -ldflags "-X main.version=...".
var version = "dev"

type rpcMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type mcpServer struct {
	mu          sync.Mutex
	out         io.Writer
	cfg         Config
	waitDefault time.Duration
	ctx         context.Context
}

func cmdMCP(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) > 0 {
		return usageError(stdout, stderr, fmt.Errorf("mcp takes no arguments"))
	}
	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintf(stderr, "harness-bridge: %v\n", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := &mcpServer{out: stdout, cfg: cfg, waitDefault: 50 * time.Second, ctx: ctx}
	var calls sync.WaitGroup
	scanner := bufio.NewScanner(stdin)
	scanner.Buffer(make([]byte, 64<<10), maxLine)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var msg rpcMessage
		if err := json.Unmarshal(line, &msg); err != nil {
			s.send(map[string]any{"jsonrpc": "2.0", "id": nil, "error": rpcError{-32700, "parse error"}})
			continue
		}
		if len(msg.ID) == 0 || msg.Method == "" {
			continue // a notification, or a reply to a request we never send
		}
		if msg.Method == "initialize" {
			s.reply(msg.ID, s.initialize(msg.Params))
			continue
		}
		calls.Add(1)
		go func() {
			defer calls.Done()
			result, rpcErr := s.dispatch(msg)
			if rpcErr != nil {
				s.send(map[string]any{"jsonrpc": "2.0", "id": msg.ID, "error": rpcErr})
				return
			}
			s.reply(msg.ID, result)
		}()
	}
	// The client closed stdin: stop waiting, but leave started jobs running.
	cancel()
	calls.Wait()
	return exitOK
}

func (s *mcpServer) send(v any) {
	data, err := json.Marshal(v)
	if err != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, _ = s.out.Write(append(data, '\n'))
}

func (s *mcpServer) reply(id json.RawMessage, result any) {
	s.send(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
}

func (s *mcpServer) initialize(params json.RawMessage) map[string]any {
	var p struct {
		ProtocolVersion string `json:"protocolVersion"`
		ClientInfo      struct {
			Name string `json:"name"`
		} `json:"clientInfo"`
	}
	_ = json.Unmarshal(params, &p)
	negotiated := mcpLatestVersion
	for _, v := range mcpVersions {
		if v == p.ProtocolVersion {
			negotiated = v
		}
	}
	// Claude Code allows long tool calls and keeps them alive with progress
	// notifications; other clients may stop a call after about a minute.
	if strings.Contains(strings.ToLower(p.ClientInfo.Name), "claude") {
		s.waitDefault = 10 * time.Minute
	}
	return map[string]any{
		"protocolVersion": negotiated,
		"capabilities":    map[string]any{"tools": map[string]any{}},
		"serverInfo":      map[string]any{"name": mcpServerName, "version": version},
		"instructions":    mcpInstructions(s.cfg),
	}
}

func (s *mcpServer) dispatch(msg rpcMessage) (any, *rpcError) {
	switch msg.Method {
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		return map[string]any{"tools": mcpTools(s.cfg)}, nil
	case "tools/call":
		var p struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
			Meta      struct {
				ProgressToken any `json:"progressToken"`
			} `json:"_meta"`
		}
		if err := json.Unmarshal(msg.Params, &p); err != nil {
			return nil, &rpcError{-32602, "invalid tools/call parameters"}
		}
		if len(p.Arguments) == 0 || string(p.Arguments) == "null" {
			p.Arguments = json.RawMessage("{}")
		}
		text, failed := s.call(p.Name, p.Arguments, p.Meta.ProgressToken)
		return map[string]any{"content": []any{map[string]any{"type": "text", "text": text}}, "isError": failed}, nil
	}
	return nil, &rpcError{-32601, "method not found: " + msg.Method}
}

// call runs one tool. A failed job is a normal result (its status says so);
// failed is true only when the call itself was wrong.
func (s *mcpServer) call(name string, args json.RawMessage, progressToken any) (text string, failed bool) {
	bad := func(err error) (string, bool) { return err.Error(), true }
	switch name {
	case "delegate":
		var in struct {
			Task           string   `json:"task"`
			Directory      string   `json:"directory"`
			Agent          string   `json:"agent"`
			SessionID      string   `json:"session_id"`
			ReadOnly       bool     `json:"read_only"`
			Model          string   `json:"model"`
			Reasoning      string   `json:"reasoning"`
			Allow          []string `json:"allow"`
			ExtraArgs      []string `json:"extra_args"`
			TimeoutMinutes float64  `json:"timeout_minutes"`
		}
		if err := json.Unmarshal(args, &in); err != nil {
			return bad(err)
		}
		req := Request{Harness: in.Agent, Dir: in.Directory, Session: in.SessionID, Sandbox: "workspace-write",
			Model: in.Model, Reasoning: in.Reasoning, Allow: in.Allow, ExtraArgs: in.ExtraArgs, Timeout: 60 * time.Minute}
		if in.ReadOnly {
			req.Sandbox = "read-only"
		}
		if in.TimeoutMinutes > 0 {
			req.Timeout = time.Duration(in.TimeoutMinutes * float64(time.Minute))
		}
		if req.Dir == "" {
			req.Dir = firstNonEmpty(os.Getenv("CLAUDE_PROJECT_DIR"), ".")
		}
		switch {
		case strings.TrimSpace(in.Task) == "":
			return bad(fmt.Errorf("task is empty"))
		case len(in.Task) > maxPromptBytes:
			return bad(fmt.Errorf("task is larger than 1 MiB"))
		}
		req.Prompt = in.Task
		applyConfig(&req)
		if err := req.validate(); err != nil {
			return bad(err)
		}
		r, err := startJob(req)
		if err != nil {
			return bad(err)
		}
		return mcpReport(r), false
	case "wait":
		var in struct {
			Job        string  `json:"job"`
			MaxSeconds float64 `json:"max_seconds"`
		}
		if err := json.Unmarshal(args, &in); err != nil {
			return bad(err)
		}
		j, err := openJob(in.Job)
		if err != nil {
			return bad(err)
		}
		limit := s.waitDefault
		if in.MaxSeconds > 0 {
			limit = min(time.Duration(in.MaxSeconds*float64(time.Second)), time.Hour)
		}
		return mcpReport(s.waitWithProgress(j, limit, progressToken)), false
	case "cancel":
		var in struct {
			Job string `json:"job"`
		}
		if err := json.Unmarshal(args, &in); err != nil {
			return bad(err)
		}
		j, err := openJob(in.Job)
		if err != nil {
			return bad(err)
		}
		if r, err := readReport(j.path(resultFile)); err == nil {
			return mcpReport(r), false
		}
		if err := os.WriteFile(j.path(cancelFile), nil, 0o600); err != nil {
			return bad(err)
		}
		return mcpReport(j.wait(s.ctx, 30*time.Second)), false
	case "route":
		var in struct {
			Request  string `json:"request"`
			Review   bool   `json:"review"`
			Provider string `json:"provider"`
		}
		if err := json.Unmarshal(args, &in); err != nil {
			return bad(err)
		}
		cfg := routeConfig{provider: firstNonEmpty(in.Provider, os.Getenv("HARNESS_BRIDGE_ROUTER"), s.cfg.Router),
			timeout: 10 * time.Second, threshold: 0.75, review: in.Review}
		if err := cfg.complete(); err != nil {
			return bad(err)
		}
		if strings.TrimSpace(in.Request) == "" {
			return bad(fmt.Errorf("request is empty"))
		}
		ctx, cancel := context.WithTimeout(s.ctx, cfg.timeout)
		defer cancel()
		return toJSON(decide(ctx, cfg, in.Request)), false
	case "agents":
		return toJSON(agentList(s.cfg)), false
	}
	return bad(fmt.Errorf("unknown tool %q", name))
}

// waitWithProgress waits like `harness-bridge wait`, sending progress
// notifications so the client does not give up on a quiet call.
func (s *mcpServer) waitWithProgress(j *job, limit time.Duration, token any) Report {
	if token == nil {
		return j.wait(s.ctx, limit)
	}
	done := make(chan Report, 1)
	go func() { done <- j.wait(s.ctx, limit) }()
	tick := time.NewTicker(progressEvery)
	defer tick.Stop()
	started := time.Now()
	for {
		select {
		case r := <-done:
			return r
		case <-tick.C:
			msg := "the job is still running"
			if st, err := readReport(j.path(statusFile)); err == nil && len(st.Recent) > 0 {
				msg = st.Recent[len(st.Recent)-1]
			}
			s.send(map[string]any{"jsonrpc": "2.0", "method": "notifications/progress", "params": map[string]any{
				"progressToken": token, "progress": int(time.Since(started).Seconds()), "message": msg,
			}})
		}
	}
}

func mcpReport(r Report) string {
	r.decorate()
	r.Next = nextHint(r, true)
	return toJSON(r)
}

func toJSON(v any) string {
	var out bytes.Buffer
	enc := json.NewEncoder(&out)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
	return strings.TrimSpace(out.String())
}

type agentInfo struct {
	Name      string   `json:"name"`
	Title     string   `json:"title"`
	Installed bool     `json:"installed"`
	Path      string   `json:"path,omitempty"`
	Role      string   `json:"role,omitempty"`
	Resume    bool     `json:"can_continue_session"`
	Notes     []string `json:"notes,omitempty"`
}

func agentList(cfg Config) []agentInfo {
	list := make([]agentInfo, 0, len(adapters))
	for _, a := range adapters {
		info := agentInfo{Name: a.Name, Title: a.Title, Resume: a.Resume, Notes: a.Notes}
		if path, ok := findBin(a.Bin); ok {
			info.Installed, info.Path = true, path
		}
		switch a.Name {
		case cfg.Planner:
			info.Role = "planner"
		case cfg.Implementer:
			info.Role = "implementer"
		}
		list = append(list, info)
	}
	return list
}

func mcpInstructions(cfg Config) string {
	impl := adapterNamed(cfg.Implementer)
	var b strings.Builder
	if impl == nil {
		b.WriteString("harness-bridge hands coding work to another coding agent. No implementer is set up yet: pass agent to delegate, or ask the user to run `harness-bridge` in a terminal to choose one.\n\n")
		impl = &adapter{Title: "the implementer", Resume: true}
	} else {
		fmt.Fprintf(&b, "harness-bridge hands coding work to %s, which implements while you plan and review.\n\n", impl.Title)
	}
	b.WriteString("For a request that changes code:\n")
	step := 1
	if cfg.Router != "" {
		fmt.Fprintf(&b, "%d. Call route with the user's request. If route is \"answer\", answer read-only. If it is \"direct_implement\", a short plan is enough.\n", step)
		step++
	}
	fmt.Fprintf(&b, "%d. Read what you need, then write a plan %s can follow without asking questions: the goal, the files to change, the steps, the checks to run and what done looks like. End it with: \"When you finish, reply with what you changed, the checks you ran and their results, and anything you could not do.\"\n", step, impl.Title)
	fmt.Fprintf(&b, "%d. Call delegate with the plan as task and directory set to the project's absolute path. Don't edit the code yourself.\n", step+1)
	fmt.Fprintf(&b, "%d. Call wait with the job id, and again while status is \"running\".\n", step+2)
	fmt.Fprintf(&b, "%d. Review the repository, not the report: run git diff and the project's checks yourself and compare the result with the plan. If git.dirty_before is above 0, the workspace already had uncommitted changes.\n", step+3)
	repair := "call delegate again with the specific fixes and the session_id from the report, so the agent continues its session"
	if !impl.Resume {
		repair = "call delegate again with the fixes and the context they need, because " + impl.Title + " cannot continue a session"
	}
	fmt.Fprintf(&b, "%d. If something is wrong, %s. Stop after 3 repair rounds.\n", step+4, repair)
	fmt.Fprintf(&b, "%d. Tell the user what changed, what you checked and how, and what is still open.\n\n", step+5)
	b.WriteString("Answer questions, explain code and review existing changes yourself, without delegating.\n")
	for _, note := range impl.Notes {
		b.WriteString("- " + note + "\n")
	}
	return clip(b.String(), mcpInstructionMax)
}

func mcpTools(cfg Config) []map[string]any {
	str := func(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }
	names := make([]string, len(adapters))
	for i, a := range adapters {
		names[i] = a.Name
	}
	implementer := "the implementer chosen in setup"
	if a := adapterNamed(cfg.Implementer); a != nil {
		implementer = a.Name + " (" + a.Title + "), chosen in setup"
	}
	object := func(props map[string]any, required ...string) map[string]any {
		schema := map[string]any{"type": "object", "properties": props}
		if len(required) > 0 {
			schema["required"] = required
		}
		return schema
	}
	return []map[string]any{
		{
			"name":        "delegate",
			"description": "Hand a coding task to another agent CLI, which edits the project. Returns at once with a job id; then call wait. Pass a complete plan as task.",
			"inputSchema": object(map[string]any{
				"task":            str("The complete plan: goal, files, steps, checks to run, and what done looks like."),
				"directory":       str("Absolute path of the project folder the agent works in."),
				"agent":           map[string]any{"type": "string", "enum": names, "description": "Agent to use. Default: " + implementer + "."},
				"session_id":      str("Continue this earlier session, to send follow-up fixes."),
				"read_only":       map[string]any{"type": "boolean", "description": "Let the agent read files but not change them."},
				"model":           str("Model for the agent."),
				"reasoning":       str("Reasoning effort for the agent."),
				"allow":           map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Claude Code only: extra tool rules such as Bash(go test:*)."},
				"extra_args":      map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Arguments passed unchanged to the agent CLI, as its notes describe."},
				"timeout_minutes": map[string]any{"type": "number", "description": "Stop the agent after this many minutes (default 60)."},
			}, "task", "directory"),
		},
		{
			"name":        "wait",
			"description": "Wait for a delegated job. Returns the result when it ends, or its latest progress when max_seconds passes first (status \"running\": call wait again). The result includes what Git sees in the workspace.",
			"inputSchema": object(map[string]any{
				"job":         str("The job id from delegate."),
				"max_seconds": map[string]any{"type": "number", "description": "How long to wait before returning progress."},
			}, "job"),
			"annotations": map[string]any{"readOnlyHint": true},
		},
		{
			"name":        "cancel",
			"description": "Stop a delegated job. Edits it already made stay in the workspace.",
			"inputSchema": object(map[string]any{"job": str("The job id from delegate.")}, "job"),
		},
		{
			"name":        "route",
			"description": "Ask the Jev or Laya decision model whether a request needs a plan (answer, needs_planning or direct_implement), or with review, whether a described change needs a full review. When the model can't decide, it returns the safe choice and says why.",
			"inputSchema": object(map[string]any{
				"request":  str("The user's request, or with review, a description of the change and its checks."),
				"review":   map[string]any{"type": "boolean", "description": "Ask about review instead of planning."},
				"provider": map[string]any{"type": "string", "enum": []string{"laya", "jev"}, "description": "Decision model. Default: the router chosen in setup."},
			}, "request"),
			"annotations": map[string]any{"readOnlyHint": true},
		},
		{
			"name":        "agents",
			"description": "List the agent CLIs harness-bridge can delegate to, whether each is installed, and the roles chosen in setup.",
			"inputSchema": object(map[string]any{}),
			"annotations": map[string]any{"readOnlyHint": true},
		},
	}
}
