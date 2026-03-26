package agent

// RunResult contains the outcome of an agent run.
type RunResult struct {
	Response  string
	ToolCalls int
}
