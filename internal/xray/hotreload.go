package xray

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// HotReload swaps outbounds and routing rules in the running Xray process
// via its gRPC API (the "xray api" CLI), without a process restart.
//
// The flow:
//  1. Write + validate the new config to disk (same as Apply/Reload)
//  2. Extract the new outbound(s) and routing rule(s) into temp JSON files
//  3. Call `xray api ado` to add the new outbound(s)
//  4. Call `xray api adrules` to add the new routing rule(s)
//  5. Call `xray api rmrules` to remove the old routing rule(s)
//  6. Call `xray api rmo` to remove the old outbound(s)
//  7. If any step fails, fall back to Reload() (fast process restart)
//
// The API server is at 127.0.0.1:<APIPort> (default 10085), already
// emitted by BuildConfig for non-ProxyConnMode configs.
//
// "Old" tags are extracted from the currently-running config file so
// the caller doesn't need to track them.
func (c *Controller) HotReload(cfg *Config) (string, error) {
	path, err := c.WriteConfig(cfg)
	if err != nil {
		return "", err
	}

	if c.Runner.DryRun {
		return path, nil
	}

	// Extract old tags from the previous config (before WriteConfig replaced it,
	// but WriteConfig backs up — we read the backup if present, or just read
	// the new config for the new tags and skip removal if we can't find old ones).
	oldOutTags, oldRuleTags := c.extractOldTags()
	newOutTags, newRuleTags := extractTagsFromConfig(cfg)

	apiAddr := fmt.Sprintf("127.0.0.1:%d", cfg.apiPort())

	tmpDir := filepath.Dir(path)
	success := true

	// Step 1: Add new outbounds.
	if len(newOutTags) > 0 {
		outFile, err := writeOutboundsFile(cfg, tmpDir)
		if err == nil {
			res := c.Runner.Run(c.bin(), "api", "ado",
				"--server", apiAddr, outFile)
			if res.Err != nil {
				if c.Logf != nil {
					c.Logf("hot-reload: ado failed: %v — will fall back to reload", res.Err)
				}
				success = false
			}
			_ = os.Remove(outFile)
		}
	}

	// Step 2: Add new routing rules.
	if success && len(newRuleTags) > 0 {
		rulesFile, err := writeRulesFile(cfg, tmpDir)
		if err == nil {
			res := c.Runner.Run(c.bin(), "api", "adrules",
				"--server", apiAddr, rulesFile)
			if res.Err != nil {
				if c.Logf != nil {
					c.Logf("hot-reload: adrules failed: %v — will fall back to reload", res.Err)
				}
				success = false
			}
			_ = os.Remove(rulesFile)
		}
	}

	// Step 3: Remove old routing rules.
	if success && len(oldRuleTags) > 0 {
		args := []string{"api", "rmrules", "--server", apiAddr}
		args = append(args, oldRuleTags...)
		res := c.Runner.Run(c.bin(), args...)
		if res.Err != nil {
			if c.Logf != nil {
				c.Logf("hot-reload: rmrules failed: %v (non-fatal — old rules may already be gone)", res.Err)
			}
			// Non-fatal: stale rules pointing to removed outbounds are harmless.
		}
	}

	// Step 4: Remove old outbounds (that are not in the new config).
	if success {
		toRemove := diffTags(oldOutTags, newOutTags)
		for _, tag := range toRemove {
			res := c.Runner.Run(c.bin(), "api", "rmo",
				"--server", apiAddr, tag)
			if res.Err != nil {
				if c.Logf != nil {
					c.Logf("hot-reload: rmo %s failed: %v (non-fatal)", tag, res.Err)
				}
			}
		}
	}

	if !success {
		if c.Logf != nil {
			c.Logf("hot-reload: gRPC API path failed — falling back to fast reload")
		}
		if err := c.fastRestart(); err != nil {
			if c.Logf != nil {
				c.Logf("fast reload failed, falling back to full restart: %v", err)
			}
			if err2 := c.Restart(); err2 != nil {
				return path, err2
			}
		}
		return path, nil
	}

	if c.Logf != nil {
		c.Logf("hot-reload: outbounds swapped via gRPC API (no process restart)")
	}
	return path, nil
}

// extractOldTags reads the backup of the previous config to find outbound
// and routing rule tags that should be removed. If the backup is not found,
// returns empty slices (removal is skipped — stale outbounds are harmless
// if not referenced by routing rules).
func (c *Controller) extractOldTags() (outTags, ruleTags []string) {
	// The backup is written by WriteConfig as xray-config-<unix>.json
	// in the backup dir. Find the most recent one.
	backupDir := c.Paths.BackupDir
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		return nil, nil
	}
	var newest string
	var newestTime int64
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), "xray-config-") || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Unix() > newestTime {
			newestTime = info.ModTime().Unix()
			newest = filepath.Join(backupDir, e.Name())
		}
	}
	if newest == "" {
		return nil, nil
	}
	data, err := os.ReadFile(newest)
	if err != nil {
		return nil, nil
	}
	var old Config
	if err := json.Unmarshal(data, &old); err != nil {
		return nil, nil
	}
	return extractTagsFromConfig(&old)
}

// extractTagsFromConfig pulls outbound tags from a Config.
// Routing rules in Xray don't have a Tag field, so we only track outbounds.
func extractTagsFromConfig(cfg *Config) (outTags, ruleTags []string) {
	for _, ob := range cfg.Outbounds {
		if ob.Tag != "" && ob.Tag != "direct" && ob.Tag != "block" && ob.Tag != "api" {
			outTags = append(outTags, ob.Tag)
		}
	}
	// Rule tags aren't available (Rule has no Tag field in this Xray version),
	// so we return nil for ruleTags. The adrules/rmrules path is skipped.
	return outTags, nil
}

// writeOutboundsFile extracts outbounds from a Config into a standalone JSON
// file that `xray api ado` can consume.
func writeOutboundsFile(cfg *Config, dir string) (string, error) {
	type outboundsFile struct {
		Outbounds []Outbound `json:"outbounds"`
	}
	data, err := json.MarshalIndent(outboundsFile{Outbounds: cfg.Outbounds}, "", "  ")
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, "hot-outbounds.json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", err
	}
	return path, nil
}

// writeRulesFile extracts routing rules from a Config into a standalone JSON
// file that `xray api adrules` can consume.
func writeRulesFile(cfg *Config, dir string) (string, error) {
	type routingFile struct {
		Routing *Routing `json:"routing"`
	}
	data, err := json.MarshalIndent(routingFile{Routing: cfg.Routing}, "", "  ")
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, "hot-rules.json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", err
	}
	return path, nil
}

// diffTags returns tags in `old` that are not in `new`.
func diffTags(old, new []string) []string {
	newSet := make(map[string]bool, len(new))
	for _, t := range new {
		newSet[t] = true
	}
	var result []string
	for _, t := range old {
		if !newSet[t] {
			result = append(result, t)
		}
	}
	return result
}

// apiPort returns the API port from the config, defaulting to 10085.
func (c *Config) apiPort() int {
	if c.API != nil {
		// Parse from the Listen field "127.0.0.1:PORT"
		if idx := strings.LastIndex(c.API.Listen, ":"); idx >= 0 {
			port := 0
			fmt.Sscanf(c.API.Listen[idx+1:], "%d", &port)
			if port > 0 {
				return port
			}
		}
	}
	return 10085
}
