package cli

import (
	"fmt"
	"time"
)

// Only bounded phase names and counts enter progress; file names and contents
// stay out of the compact display and structured logs.
func (p *progressSink) WorkspaceInspection(phase string, files int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if phase == "" {
		p.view.workspaceScan = ""
		return
	}
	switch phase {
	case "listing files (pass 1/2)", "listing files (pass 2/2)", "reading files (pass 1/2)", "reading files (pass 2/2)", "hashing files (pass 1/2)", "hashing files (pass 2/2)":
	default:
		return
	}
	if files < 0 {
		return
	}
	first := p.view.workspaceScan == ""
	p.flushActivity(time.Now())
	p.view.workspaceScan = fmt.Sprintf("%s · %d files", phase, files)
	if first && !p.quiet && !p.view.modal && !p.view.paused && p.format == "text" {
		p.clearLine()
		p.writeBytes([]byte("[INFO] Inspecting workspace files before continuing; large folders may take longer.\n"))
	}
	p.drawLive(time.Now())
}
