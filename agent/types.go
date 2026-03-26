package agent

// RunResult 包含 Agent 运行的结果。
type RunResult struct {
	Response  string
	ToolCalls int
}
