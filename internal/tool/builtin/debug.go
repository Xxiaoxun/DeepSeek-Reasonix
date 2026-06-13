package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"reasonix/internal/tool"
)

// sessionIDPattern restricts sessionId to a safe identifier set so the value
// can be joined into a filesystem path without enabling traversal — both
// debug log files and the todo store derive paths from caller-supplied IDs.
var sessionIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

func validateSessionID(id string) error {
	if id == "" {
		return fmt.Errorf("sessionId is required")
	}
	if !sessionIDPattern.MatchString(id) {
		return fmt.Errorf("sessionId %q contains invalid characters (allowed: A-Z a-z 0-9 _ -, max 64)", id)
	}
	return nil
}

func init() {
	tool.RegisterBuiltin(debugInstrument{})
	tool.RegisterBuiltin(debugReadLog{})
	tool.RegisterBuiltin(debugClear{})
}

// ─── Debug Session Manager ──────────────────────────────────────────

type debugSession struct {
	ID        string
	LogPath   string
	CreatedAt time.Time
}

var (
	debugSessions   = make(map[string]*debugSession)
	debugSessionsMu sync.RWMutex
)

func getOrCreateSession(sessionID string) *debugSession {
	debugSessionsMu.Lock()
	defer debugSessionsMu.Unlock()
	if s, ok := debugSessions[sessionID]; ok {
		return s
	}
	s := &debugSession{
		ID:        sessionID,
		LogPath:   filepath.Join(".reasonix-debug", sessionID+".log"),
		CreatedAt: time.Now(),
	}
	os.MkdirAll(filepath.Dir(s.LogPath), 0o755)
	debugSessions[sessionID] = s
	return s
}

func removeSession(sessionID string) {
	debugSessionsMu.Lock()
	defer debugSessionsMu.Unlock()
	if s, ok := debugSessions[sessionID]; ok {
		os.Remove(s.LogPath)
		delete(debugSessions, sessionID)
	}
}

// ─── debug_instrument ───────────────────────────────────────────────

type debugInstrument struct{}

func (debugInstrument) Name() string { return "debug_instrument" }

func (debugInstrument) Description() string {
	return `Add temporary debug instrumentation to a file. Creates a new function with logging statements that write to the debug log file. Use this to trace execution flow, variable values, and timing without modifying the original code directly. The instrumented code is wrapped in #region markers for easy cleanup.`
}

func (debugInstrument) Schema() json.RawMessage {
	return json.RawMessage(`{
"type": "object",
"properties": {
  "sessionId": {
	"type": "string",
	"description": "Debug session ID for grouping related instrumentation."
  },
  "filePath": {
	"type": "string",
	"description": "Path to the file to instrument."
  },
  "functionName": {
	"type": "string",
	"description": "Name of the function or code block to instrument."
  },
  "logPoints": {
	"type": "array",
	"description": "Points to add logging.",
	"items": {
	  "type": "object",
	  "properties": {
		"location": {
		  "type": "string",
		  "description": "Code location (e.g., 'before loop', 'after assignment')."
		},
		"variables": {
		  "type": "array",
		  "items": { "type": "string" },
		  "description": "Variables to log at this point."
		},
		"message": {
		  "type": "string",
		  "description": "Custom log message."
		}
	  },
	  "required": ["location"]
	}
  },
  "hypothesisId": {
	"type": "string",
	"description": "Hypothesis ID this instrumentation is testing (e.g., 'H1')."
  }
},
"required": ["sessionId", "filePath", "logPoints"]
}`)
}

func (debugInstrument) ReadOnly() bool { return false }

func (debugInstrument) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		SessionID    string `json:"sessionId"`
		FilePath     string `json:"filePath"`
		FunctionName string `json:"functionName"`
		LogPoints    []struct {
			Location  string   `json:"location"`
			Variables []string `json:"variables,omitempty"`
			Message   string   `json:"message,omitempty"`
		} `json:"logPoints"`
		HypothesisID string `json:"hypothesisId"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid args: %w", err)
	}

	if err := validateSessionID(p.SessionID); err != nil {
		return "", err
	}
	if p.FilePath == "" {
		return "", fmt.Errorf("filePath is required")
	}
	if len(p.LogPoints) == 0 {
		return "", fmt.Errorf("at least one logPoint is required")
	}

	s := getOrCreateSession(p.SessionID)

	// Build the instrumentation code
	var b strings.Builder
	b.WriteString(fmt.Sprintf("// #region reasonix-debug %s\n", p.SessionID))
	b.WriteString(fmt.Sprintf("// Debug instrumentation for hypothesis %s\n", p.HypothesisID))
	b.WriteString(fmt.Sprintf("// File: %s\n", p.FilePath))
	b.WriteString(fmt.Sprintf("// Generated: %s\n\n", time.Now().Format(time.RFC3339)))

	b.WriteString("/*\n")
	b.WriteString("  To use this instrumentation:\n")
	b.WriteString("  1. Copy this function to your code\n")
	b.WriteString("  2. Call it at the appropriate location\n")
	b.WriteString("  3. Run your code to generate logs\n")
	b.WriteString("  4. Use debug_read_log to analyze results\n")
	b.WriteString("*/\n\n")

	funcName := p.FunctionName
	if funcName == "" {
		funcName = "debugTrace"
	}

	b.WriteString(fmt.Sprintf("func %s_debug_%s() {\n", funcName, p.SessionID))
	b.WriteString(fmt.Sprintf(`	logFile := %q
	f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()

	writeLog := func(msg string) {
		entry := map[string]interface{}{
			"timestamp": time.Now().Format(time.RFC3339Nano),
			"sessionId": %q,
			"hypothesisId": %q,
			"file": %q,
			"message": msg,
		}
		jsonBytes, _ := json.Marshal(entry)
		f.Write(append(jsonBytes, '\n'))
	}

`, s.LogPath, p.SessionID, p.HypothesisID, p.FilePath))

	for i, lp := range p.LogPoints {
		msg := lp.Message
		if msg == "" {
			msg = fmt.Sprintf("Log point %d: %s", i+1, lp.Location)
		}
		if len(lp.Variables) > 0 {
			varParts := make([]string, len(lp.Variables))
			for j, v := range lp.Variables {
				varParts[j] = fmt.Sprintf("%s=%%v", v)
			}
			b.WriteString(fmt.Sprintf(`	// %s
	writeLog(fmt.Sprintf("%s: %s", %s))
`, lp.Location, msg, strings.Join(varParts, ", "), strings.Join(lp.Variables, ", ")))
		} else {
			b.WriteString(fmt.Sprintf(`	// %s
	writeLog(%q)
`, lp.Location, msg))
		}
	}

	b.WriteString("}\n")
	b.WriteString(fmt.Sprintf("// #endregion reasonix-debug %s\n", p.SessionID))

	return fmt.Sprintf("Debug instrumentation created for session %s.\n\nGenerated code:\n%s\n\nLog file: %s\n\nTo use:\n1. Add this function to your code\n2. Call it at the location you want to trace\n3. Run your code\n4. Use debug_read_log(sessionId=%q) to read results",
		p.SessionID, b.String(), s.LogPath, p.SessionID), nil
}

// ─── debug_read_log ─────────────────────────────────────────────────

type debugReadLog struct{}

func (debugReadLog) Name() string { return "debug_read_log" }

func (debugReadLog) Description() string {
	return `Read and analyze debug log entries for a session. Returns structured log entries that can be used to verify or reject hypotheses. Supports filtering by hypothesis ID and time range.`
}

func (debugReadLog) Schema() json.RawMessage {
	return json.RawMessage(`{
"type": "object",
"properties": {
  "sessionId": {
	"type": "string",
	"description": "Debug session ID to read logs for."
  },
  "hypothesisId": {
	"type": "string",
	"description": "Filter logs by hypothesis ID (optional)."
  },
  "limit": {
	"type": "integer",
	"description": "Maximum number of entries to return (default: 100)."
  }
},
"required": ["sessionId"]
}`)
}

func (debugReadLog) ReadOnly() bool { return true }

func (debugReadLog) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		SessionID    string `json:"sessionId"`
		HypothesisID string `json:"hypothesisId,omitempty"`
		Limit        int    `json:"limit,omitempty"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid args: %w", err)
	}

	if err := validateSessionID(p.SessionID); err != nil {
		return "", err
	}
	if p.Limit <= 0 {
		p.Limit = 100
	}

	logPath := filepath.Join(".reasonix-debug", p.SessionID+".log")
	data, err := os.ReadFile(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Sprintf("No log file found for session %s. The instrumentation may not have been executed yet.", p.SessionID), nil
		}
		return "", fmt.Errorf("read log: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	var entries []map[string]interface{}
	for _, line := range lines {
		if line == "" {
			continue
		}
		var entry map[string]interface{}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if p.HypothesisID != "" {
			if hid, ok := entry["hypothesisId"].(string); ok && hid != p.HypothesisID {
				continue
			}
		}
		entries = append(entries, entry)
		if len(entries) >= p.Limit {
			break
		}
	}

	if len(entries) == 0 {
		return fmt.Sprintf("No log entries found for session %s (hypothesis: %s).", p.SessionID, p.HypothesisID), nil
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("Debug Log Analysis - Session: %s\n", p.SessionID))
	b.WriteString(fmt.Sprintf("Total entries: %d\n\n", len(entries)))

	for i, entry := range entries {
		b.WriteString(fmt.Sprintf("--- Entry %d ---\n", i+1))
		for k, v := range entry {
			b.WriteString(fmt.Sprintf("  %s: %v\n", k, v))
		}
		b.WriteString("\n")
	}

	b.WriteString("Hypothesis Verification:\n")
	b.WriteString("  Review the entries above to verify your hypothesis.\n")
	b.WriteString("  - CONFIRMED: Log evidence supports the hypothesis\n")
	b.WriteString("  - REJECTED: Log evidence contradicts the hypothesis\n")
	b.WriteString("  - INCONCLUSIVE: Need more data or different instrumentation\n")

	return b.String(), nil
}

// ─── debug_clear ────────────────────────────────────────────────────

type debugClear struct{}

func (debugClear) Name() string { return "debug_clear" }

func (debugClear) Description() string {
	return `Clean up debug instrumentation for a session. Removes the log file and provides instructions for removing #region markers from source files. Call this after debugging is complete.`
}

func (debugClear) Schema() json.RawMessage {
	return json.RawMessage(`{
"type": "object",
"properties": {
  "sessionId": {
	"type": "string",
	"description": "Debug session ID to clean up."
  },
  "confirm": {
	"type": "boolean",
	"description": "Confirm deletion of log files."
  }
},
"required": ["sessionId"]
}`)
}

func (debugClear) ReadOnly() bool { return false }

func (debugClear) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		SessionID string `json:"sessionId"`
		Confirm   bool   `json:"confirm"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid args: %w", err)
	}

	if err := validateSessionID(p.SessionID); err != nil {
		return "", err
	}

	logPath := filepath.Join(".reasonix-debug", p.SessionID+".log")

	if !p.Confirm {
		return fmt.Sprintf("Debug session %s cleanup preview:\n\nLog file to remove: %s\n\nTo remove #region markers from source files:\n1. Search for '#region reasonix-debug %s'\n2. Delete everything until '#endregion reasonix-debug %s'\n\nCall this tool again with confirm=true to delete the log file.",
			p.SessionID, logPath, p.SessionID, p.SessionID), nil
	}

	// Remove log file
	if err := os.Remove(logPath); err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("remove log: %w", err)
	}

	// Remove session from memory
	removeSession(p.SessionID)

	return fmt.Sprintf("Debug session %s cleaned up.\n\nRemaining steps:\n1. Search your codebase for '#region reasonix-debug %s'\n2. Remove all instrumented code blocks\n3. Remove any debug functions you added", p.SessionID, p.SessionID), nil
}
