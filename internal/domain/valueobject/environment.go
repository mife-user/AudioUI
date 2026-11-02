package valueobject

import "time"

// Layout describes the portable directory layout beside the executable.
// WorkspaceDir and SessionsFile are M1 legacy paths kept for migration and
// for the still-active M1 session service.
type Layout struct {
	Root         string
	DataDir      string
	LogsDir      string
	WebView2Dir  string
	LogFile      string
	RootSource   string
	Portable     bool
	AgentDir     string
	WorkspaceDir string
	SessionsFile string
	AuditFile    string
	ConfigFile   string
	ProjectsDir  string
	RuntimeDir   string
	BackupDir    string
}

// CLIStatus is the result of probing the Claude Code CLI.
type CLIStatus struct {
	Installed bool
	Path      string
	Version   string
	Error     string
}

// WebView2Status is the result of probing the WebView2 runtime.
type WebView2Status struct {
	Installed bool
	Path      string
	Version   string
	Error     string
}

// EnvironmentInfo is the startup environment shown in the M0 self-check shell.
type EnvironmentInfo struct {
	Version   string
	Layout    Layout
	CLI       CLIStatus
	WebView2  WebView2Status
	CheckedAt time.Time
}

// LogEntry is one line from the local application log.
type LogEntry struct {
	Time    string
	Level   string
	Message string
}
