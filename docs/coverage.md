# Coverage after cleanup

Measured on 2026-09-05 on macOS arm64 with Go 1.26.6. Both measurements use
`go test -count=1 -timeout 10m -coverprofile=... ./...`, with live-agent and runtime
probe opt-ins disabled. These measurements include the test cleanup and removal
of redundant production files, before the later adapter consolidations and
package renames. Source line counts below also precede the codebase formatting
pass. Run `make coverage` for a fresh profile of the latest checkout.

| Area | Before test cleanup | Measured suite |
| --- | ---: | ---: |
| **All packages, statement-weighted** | **86.3%** | **81.8%** |
| Workflow core | 94.3% | 92.0% |
| Provider failures | 95.0% | 95.0% |
| Codex adapter | 82.1% | 78.6% |
| OpenCode adapter | 88.3% | 83.9% |
| Shared structured responses | 61.0% | 72.8% |
| Store contracts | 80.9% | 80.9% |
| Composition root | 71.7% | 59.8% |
| Configuration | 93.9% | 90.7% |
| CLI transport | 85.2% | 70.3% |
| Agent activity | 97.3% | 58.9% |
| Process execution | 91.9% | 88.2% |
| Dependency setup | 83.0% | 83.0% |
| Deterministic validation | 96.7% | 96.7% |
| Git workspace evidence | 82.4% | 82.4% |

Covered statements changed from 2,928/3,391 to 2,768/3,385.
The lower percentage is expected: tests of peripheral presentation, constructors,
private helpers and duplicate acceptance paths were removed. We did not disable
production packages in the coverage command or add low-value tests to recover the
old percentage. The earlier architecture refactor raised overall coverage from
84.5% to 86.3%; test cleanup reduced it to 81.3%. Removing forwarding code and
moving parser tests to their owning package brought it to the current 81.8%.

Go test source fell from **75 files / 10,391 lines** to **62 files / 8,436 lines**.
The six Gherkin feature files and eight Cucumber-related dependencies were also
removed. The replacement integration suite has 10 workflow and 12 provider-failure
scenarios. Provider classification, bounded retries, consent, cancellation,
repository preservation, strict decisions and review/repair behavior retain their
focused tests. The subsequent source cleanup removed eight production files and
99 lines (113 files / 8,863 lines → 105 files / 8,764 lines).

These are statement coverage figures, not branch coverage or release completeness.
Each package is instrumented for its own test binary. Calls into shared helpers
from another package's tests are not credited here; `-coverpkg=./...` measures that
separately and should not be compared directly to this table.

## Reproduce

```sh
make coverage
```

The target forces live calls off and regenerates ignored local reports, which
were removed from the workspace during file cleanup:

- `.coverage/coverage.out`: Go coverage profile.
- `.coverage/index.html`: source highlighting for covered and uncovered code.

`go tool cover -func=.coverage/coverage.out` prints the function summary without
rerunning tests. Test purpose and commands are in [testing.md](testing.md).

Context tests verify self-contained repair prompts after simulated loss of harness
history. There is no application-owned compactor, and native Codex/OpenCode
compaction was not exercised. Authenticated end-to-end approval, real provider
lifecycle behavior, fresh-host installation and remote CI remain separate
[release gates](release-readiness.md). No model or installer calls ran here.
