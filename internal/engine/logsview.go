package engine

import "github.com/miroslavrov/keen-manager/internal/platform"

// Logs returns up to `lines` recent log lines for a service. keen-manager's own
// log comes from the in-memory ring (and is also streamed live over SSE); the
// managed services are tailed from their log files on demand.
func (e *Engine) Logs(service string, lines int) LogView {
	if lines <= 0 || lines > 5000 {
		lines = 300
	}
	switch service {
	case "", "keen-manager":
		return LogView{Service: "keen-manager", Lines: e.logs.tail(lines)}
	case "xray":
		return LogView{Service: "xray", Lines: tailFile(e.Paths.LogFile("xray"), lines)}
	case "nfqws2":
		return LogView{Service: "nfqws2", Lines: tailFile(e.Paths.LogFile("nfqws2"), lines)}
	case "awg":
		// AmneziaWG has no log file; surface a live `awg show` snapshot instead.
		if out, err := e.runner.Output(e.awgShowBin(), "show"); err == nil && out != "" {
			return LogView{Service: "awg", Lines: splitLines(out)}
		}
		return LogView{Service: "awg", Lines: []string{}}
	default:
		return LogView{Service: service, Lines: []string{}}
	}
}

func (e *Engine) awgShowBin() string {
	if platform.FileExists(e.Paths.AwgBin) {
		return e.Paths.AwgBin
	}
	return "awg"
}

func splitLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}
