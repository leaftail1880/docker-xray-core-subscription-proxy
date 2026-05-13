package xray

import (
	"os"
	"regexp"
	"strings"
	"time"

	"xray-balancer/pkg/logger"

	xlog "github.com/xtls/xray-core/common/log"
	"github.com/xtls/xray-core/core"
)

func Start(config *core.Config) (*core.Instance, error) {
	server, err := core.New(config)
	if err != nil {
		return nil, err
	}

	xlog.RegisterHandler(&coloredHandler{})

	if err := server.Start(); err != nil {
		return nil, err
	}
	return server, nil
}

func Restart(old *core.Instance, newConfig *core.Config) (*core.Instance, error) {
	logger.Info.Println("Restarting Xray core...")
	if old != nil {
		old.Close()
	}
	return Start(newConfig)
}

type coloredHandler struct{}

// Base ANSI colour codes (normal intensity) for each severity
var baseColor = map[xlog.Severity]string{
	xlog.Severity_Error:   "31",
	xlog.Severity_Unknown: "31",
	xlog.Severity_Warning: "33",
	xlog.Severity_Debug:   "36",
	xlog.Severity_Info:    "32",
}

// Level labels
var levelLabel = map[xlog.Severity]string{
	xlog.Severity_Error:   "ERROR",
	xlog.Severity_Unknown: "ERROR",
	xlog.Severity_Warning: "WARN",
	xlog.Severity_Debug:   "DEBUG",
	xlog.Severity_Info:    "INFO",
}

// 24‑bit RGB colours for highlighted addresses (extremely bright)
var highlightRGB = map[xlog.Severity]string{
	xlog.Severity_Error:   "38;2;255;80;80", // bright red
	xlog.Severity_Unknown: "38;2;255;80;80",
	xlog.Severity_Warning: "38;2;255;255;240", // bright yellow
	xlog.Severity_Debug:   "38;2;240;255;255", // bright cyan
	xlog.Severity_Info:    "38;2;240;255;240", // bright green
}

// addrPattern matches IPv4, IPv6, hostnames, optional port, and optional tcp:/udp: prefix
var addrPattern = regexp.MustCompile(
	`\b(?:tcp:|udp:)?` + // optional protocol
		`(?:` +
		`\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}` + // IPv4
		`|` +
		`\[[0-9a-fA-F:]+\]` + // bracketed IPv6 (must have brackets)
		`|` +
		`[0-9a-fA-F]+:+[0-9a-fA-F:]*` + // unbracketed IPv6 (must contain at least one colon)
		`|` +
		`[a-zA-Z0-9](?:[a-zA-Z0-9-]*[a-zA-Z0-9])?` + // hostname
		`(?:\.[a-zA-Z0-9](?:[a-zA-Z0-9-]*[a-zA-Z0-9])?)*\.[a-zA-Z]{2,}` +
		`)` +
		`(:\d+)?\b`, // optional port
)

func (h *coloredHandler) Handle(msg xlog.Message) {
	// Determine severity; default to Info
	severity := xlog.Severity_Info
	if gm, ok := msg.(*xlog.GeneralMessage); ok {
		severity = gm.Severity
	}

	// Get the raw message without Xray's own [Severity] prefix
	content := msg.String()
	if idx := strings.Index(content, "] "); idx != -1 {
		content = content[idx+2:]
	}

	color := baseColor[severity]
	label := levelLabel[severity]
	timestamp := time.Now().Format("2006.01.02 15:04:05")

	// Highlight addresses in the message with extreme RGB brightness
	brightMsg := addrPattern.ReplaceAllStringFunc(content, func(match string) string {
		return "\033[0;" + color + "m\033[" + highlightRGB[severity] + "m" + match +
			"\033[0;" + color + "m"
	})

	// Build the final line: bold [LEVEL] timestamp, then message with highlights
	// \033[1;colorm – bold + color
	// \033[0;colorm – reset bold, keep color
	line := "\033[1;" + color + "m[" + label + "] " + timestamp +
		"\033[0;" + color + "m " + brightMsg + "\033[0m"

	out := os.Stdout
	if severity == xlog.Severity_Error || severity == xlog.Severity_Unknown {
		out = os.Stderr
	}
	out.WriteString(line + "\n")
}
