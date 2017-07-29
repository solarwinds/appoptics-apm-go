package traceview

type AgentMetrics interface {
	FlushBSON() [][]byte
}
