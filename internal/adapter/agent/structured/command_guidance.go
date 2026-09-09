package structured

// Shared execution guidance keeps agent command evidence consistent across roles.
// It does not rewrite commands or relax each role's read/write permissions.
const commandEvidenceInstructions = `

Keep the user informed with brief public progress messages before substantial investigation and when findings change your approach. Inspect only the projects and files relevant to the request; do not recursively dump the entire workspace to answer a simple question.
When a shell command contains multiple dependent operations or pipelines, propagate failures explicitly (for Bash, use set -e and set -o pipefail where appropriate, or check each exit status). A successful final pipeline command does not prove earlier operations succeeded. Read command output, distinguish missing tools/dependencies from code failures, and report any checks that could not run. Never treat a directory listing or an agent summary as validation evidence.
`
