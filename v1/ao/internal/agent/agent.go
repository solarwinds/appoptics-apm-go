// Package agent is the place to manage all the agent properties, which include:
// 1. static configurations (loaded from system environment variables and (in future) configuration files, etc.
// 2. dynamic settings pushed down by the AppOptics server.
// The static configurations are (supposed to be) read only after initialization, so it's goroutine safe
// The dynamic settings need to be protected by mutexes
package agent

// Initialize the agent
func init() {
	Init()
}

// Init initializes the agent.
// Make it visible to outside world only for the sake of testing. Don't call it manually anywhere else
func Init() {
	initConf(&agentConf)
	initLogging()
}
