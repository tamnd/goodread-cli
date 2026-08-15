package cli

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// The config file, ~/.config/goodread/config.toml.
//
// Every key here has a flag and the flag wins, because a run that behaves
// differently on two machines with the same command line is a run nobody can
// help you debug. The order is flag, then environment, then file, then default.
//
// Read by hand rather than with a TOML library. The whole grammar this file
// needs is `key = value` and `# comment`, and a dependency that also brings
// arrays of tables and datetimes and dotted keys would be inviting a config
// file that the flags cannot express. If a key cannot be a flag it does not
// belong here, and the parser saying so is a feature.
//
// What is deliberately absent is `no_robots`. The override's whole
// justification is that a person decided, this invocation, that it was their
// call, and a config key would make it ambient. TestNoRobotsNotInConfigFile
// holds that, and an attempt to set it is an error rather than a silent
// ignore, so nobody spends an afternoon wondering why it does nothing.

// configPath is where the file lives.
func configPath() string {
	if p := os.Getenv("GOODREAD_CONFIG"); p != "" {
		return p
	}
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "goodread", "config.toml")
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".config", "goodread", "config.toml")
}

// fileConfig is one parsed config file.
//
// Every field is a pointer so that "not in the file" and "in the file, set to
// the zero value" stay apart. `cache = false` has to be able to turn something
// off that defaults on, and a bool cannot say that on its own.
type fileConfig struct {
	pace      *time.Duration
	depth     *string
	cacheTTL  *time.Duration
	noCache   *bool
	store     *string
	dataDir   *string
	userAgent *string
	format    *string
	timeout   *time.Duration
	retries   *int
}

// configKeys maps every key to the flag that overrides it.
//
// One table rather than a switch, so that "every key has a flag" is a thing the
// code states and a test can check rather than a promise in a document.
var configKeys = map[string]string{
	"pace":       "delay",
	"depth":      "depth",
	"cache_ttl":  "cache-ttl",
	"no_cache":   "no-cache",
	"store":      "store",
	"data_dir":   "data-dir",
	"user_agent": "user-agent",
	"format":     "format",
	"timeout":    "timeout",
	"retries":    "retries",
}

// loadConfigFile reads the file, or returns nothing if there is not one.
//
// A missing file is not an error. A file that is there and does not parse is,
// because the alternative is running with settings the user thinks they
// changed.
func loadConfigFile(path string) (*fileConfig, error) {
	if path == "" {
		return nil, nil
	}
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	fc := &fileConfig{}
	sc := bufio.NewScanner(f)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, raw, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("%s:%d: %q is not key = value", path, n, line)
		}
		key = strings.TrimSpace(key)
		val := strings.Trim(strings.TrimSpace(raw), `"'`)

		if key == "no_robots" {
			return nil, fmt.Errorf(
				"%s:%d: no_robots is not a config key. it is a decision for one "+
					"invocation, so pass --no-robots on the command line", path, n)
		}
		if _, known := configKeys[key]; !known {
			return nil, fmt.Errorf("%s:%d: unknown key %q", path, n, key)
		}

		switch key {
		case "pace":
			d, err := time.ParseDuration(val)
			if err != nil {
				return nil, fmt.Errorf("%s:%d: pace %q is not a duration", path, n, val)
			}
			fc.pace = &d
		case "cache_ttl":
			d, err := time.ParseDuration(val)
			if err != nil {
				return nil, fmt.Errorf("%s:%d: cache_ttl %q is not a duration", path, n, val)
			}
			fc.cacheTTL = &d
		case "timeout":
			d, err := time.ParseDuration(val)
			if err != nil {
				return nil, fmt.Errorf("%s:%d: timeout %q is not a duration", path, n, val)
			}
			fc.timeout = &d
		case "retries":
			i, err := strconv.Atoi(val)
			if err != nil || i < 0 {
				return nil, fmt.Errorf("%s:%d: retries %q is not a count", path, n, val)
			}
			fc.retries = &i
		case "no_cache":
			b, err := strconv.ParseBool(val)
			if err != nil {
				return nil, fmt.Errorf("%s:%d: no_cache %q is not true or false", path, n, val)
			}
			fc.noCache = &b
		case "depth":
			v := val
			fc.depth = &v
		case "store":
			v := expandHome(val)
			fc.store = &v
		case "data_dir":
			v := expandHome(val)
			fc.dataDir = &v
		case "user_agent":
			v := val
			fc.userAgent = &v
		case "format":
			v := val
			fc.format = &v
		}
	}
	return fc, sc.Err()
}

// expandHome turns a leading ~ into the home directory.
//
// The spec's example config writes `store = "~/.local/share/goodread/books.db"`
// and a shell would have expanded that before the flag ever saw it. Reading it
// literally would create a directory called ~ in whatever the cwd happens to be.
func expandHome(p string) string {
	if p != "~" && !strings.HasPrefix(p, "~/") {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return p
	}
	return filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(p, "~"), "/"))
}

// applyConfigFile fills in the settings the command line did not.
//
// Every one of these asks Changed first. A flag the user typed wins even when
// it happens to equal the default, because they typed it.
func (a *App) applyConfigFile(cmd *cobra.Command, fc *fileConfig) {
	if fc == nil {
		return
	}
	f := cmd.Flags()
	set := func(name string) bool { return f.Changed(name) }

	if fc.pace != nil && !set("delay") {
		a.cfg.Delay = *fc.pace
	}
	if fc.cacheTTL != nil && !set("cache-ttl") {
		a.cfg.CacheTTL = *fc.cacheTTL
	}
	if fc.timeout != nil && !set("timeout") {
		a.cfg.Timeout = *fc.timeout
	}
	if fc.retries != nil && !set("retries") {
		a.cfg.Retries = *fc.retries
	}
	if fc.noCache != nil && !set("no-cache") {
		a.cfg.NoCache = *fc.noCache
	}
	if fc.dataDir != nil && !set("data-dir") {
		a.cfg.DataDir = *fc.dataDir
	}
	if fc.userAgent != nil && !set("user-agent") {
		a.cfg.UserAgent = *fc.userAgent
	}
	if fc.depth != nil && !set("depth") {
		a.depthArg = *fc.depth
	}
	if fc.store != nil && !set("store") {
		a.storePtr = *fc.store
	}
	// format is the one that also answers to a TTY check, and the file sits
	// between the two: it beats the guess and loses to the flag.
	if fc.format != nil && !set("format") && !set("json") {
		a.format = *fc.format
	}
}
