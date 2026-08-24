package main

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Config holds the plugin's user-tunable settings, read from
// $HERDR_PLUGIN_CONFIG_DIR/config.toml.
type Config struct {
	// Ratio is the fraction of the tab given to the focused pane.
	Ratio float64
	// Auto enables the pane.focused event hook. Off by default: the hook runs
	// a fresh process on every focus change, so it is opt-in.
	Auto bool
	// Debounce is how long the hook waits before acting, to coalesce bursts
	// of focus changes.
	Debounce time.Duration
	// MinDelta suppresses a resize when the split is already this close to
	// the target, avoiding redundant no-op writes.
	MinDelta float64
}

func defaultConfig() Config {
	return Config{
		Ratio:    Golden,
		Auto:     false,
		Debounce: 120 * time.Millisecond,
		MinDelta: 0.02,
	}
}

// LoadConfig reads config.toml from the plugin's config directory. Missing file,
// unreadable file and unknown keys are all non-fatal: defaults are used so a
// keypress never fails because of a typo in the config.
func LoadConfig() Config {
	cfg := defaultConfig()

	dir := os.Getenv("HERDR_PLUGIN_CONFIG_DIR")
	if dir == "" {
		return cfg
	}
	f, err := os.Open(filepath.Join(dir, "config.toml"))
	if err != nil {
		return cfg
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		key, val, ok := parseAssignment(sc.Text())
		if !ok {
			continue
		}
		switch key {
		case "ratio":
			if v, err := strconv.ParseFloat(val, 64); err == nil && v > 0 && v < 1 {
				cfg.Ratio = v
			}
		case "auto":
			if v, err := strconv.ParseBool(val); err == nil {
				cfg.Auto = v
			}
		case "debounce_ms":
			if v, err := strconv.Atoi(val); err == nil && v >= 0 {
				cfg.Debounce = time.Duration(v) * time.Millisecond
			}
		case "min_delta":
			if v, err := strconv.ParseFloat(val, 64); err == nil && v >= 0 {
				cfg.MinDelta = v
			}
		}
	}
	return cfg
}

// parseAssignment pulls a `key = value` pair out of one line, ignoring comments,
// blank lines and TOML table headers. Quotes around the value are stripped.
func parseAssignment(line string) (key, val string, ok bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "[") {
		return "", "", false
	}
	// Strip a trailing comment. Values here are numbers and booleans, so a '#'
	// can be treated as a comment start without worrying about quoting.
	if i := strings.Index(line, "#"); i >= 0 {
		line = line[:i]
	}
	eq := strings.Index(line, "=")
	if eq < 0 {
		return "", "", false
	}
	key = strings.TrimSpace(line[:eq])
	val = strings.Trim(strings.TrimSpace(line[eq+1:]), `"'`)
	if key == "" || val == "" {
		return "", "", false
	}
	return key, val, true
}
