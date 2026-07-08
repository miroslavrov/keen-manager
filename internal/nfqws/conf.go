// Package nfqws integrates the existing nfqws2-keenetic package: it drives the
// init script and edits the shell config + hostlists, rather than reimplementing
// the DPI-bypass daemon. This keeps the (unlicensed upstream) nfqws2 binary out
// of keen-manager entirely.
package nfqws

import (
	"os"
	"regexp"
	"strings"

	"github.com/miroslavrov/keen-manager/internal/model"
)

var extraArgsRe = regexp.MustCompile(`(?m)^\s*NFQWS_EXTRA_ARGS\s*=\s*(.+?)\s*$`)

// ReadConfigRaw returns the raw nfqws2.conf text.
func (c *Controller) ReadConfigRaw() (string, error) {
	b, err := os.ReadFile(c.confFile())
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// WriteConfigRaw overwrites nfqws2.conf (a backup is written first).
func (c *Controller) WriteConfigRaw(text string) error {
	path := c.confFile()
	if old, err := os.ReadFile(path); err == nil {
		_ = os.WriteFile(path+".keen.bak", old, 0o600)
	}
	return os.WriteFile(path, []byte(text), 0o644)
}

// DetectMode reads the active mode from the config text.
func DetectMode(conf string) model.NfqwsMode {
	m := extraArgsRe.FindStringSubmatch(conf)
	if m == nil {
		return model.ModeAuto
	}
	v := m[1]
	switch {
	case strings.Contains(v, "MODE_AUTO"):
		return model.ModeAuto
	case strings.Contains(v, "MODE_LIST"):
		return model.ModeList
	case strings.Contains(v, "MODE_ALL"):
		return model.ModeAll
	}
	return model.ModeAuto
}

// SetMode rewrites NFQWS_EXTRA_ARGS to the given macro, returning the new text.
func SetMode(conf string, mode model.NfqwsMode) string {
	repl := `NFQWS_EXTRA_ARGS="$` + string(mode) + `"`
	if extraArgsRe.MatchString(conf) {
		return extraArgsRe.ReplaceAllString(conf, repl)
	}
	// Append if the key is missing.
	if !strings.HasSuffix(conf, "\n") {
		conf += "\n"
	}
	return conf + repl + "\n"
}

// Mode reads the current mode from disk.
func (c *Controller) Mode() (model.NfqwsMode, error) {
	raw, err := c.ReadConfigRaw()
	if err != nil {
		return model.ModeAuto, err
	}
	return DetectMode(raw), nil
}

// SetModeOnDisk updates just the mode in nfqws2.conf.
func (c *Controller) SetModeOnDisk(mode model.NfqwsMode) error {
	raw, err := c.ReadConfigRaw()
	if err != nil {
		return err
	}
	return c.WriteConfigRaw(SetMode(raw, mode))
}
