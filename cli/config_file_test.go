package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestConfigFileReadsTheSpecExample(t *testing.T) {
	p := writeConfig(t, `# the example from 05_commands.md
pace = "2s"
depth = "meta"
cache_ttl = "24h"
store = "~/.local/share/goodread/books.db"
user_agent = "goodread/0.3.0 (+https://github.com/tamnd/goodread-cli)"
`)
	fc, err := loadConfigFile(p)
	if err != nil {
		t.Fatalf("loadConfigFile: %v", err)
	}
	if fc.pace == nil || *fc.pace != 2*time.Second {
		t.Errorf("pace = %v", fc.pace)
	}
	if fc.depth == nil || *fc.depth != "meta" {
		t.Errorf("depth = %v", fc.depth)
	}
	if fc.cacheTTL == nil || *fc.cacheTTL != 24*time.Hour {
		t.Errorf("cache_ttl = %v", fc.cacheTTL)
	}
	if fc.store == nil || strings.HasPrefix(*fc.store, "~") {
		t.Errorf("store = %v, and a literal ~ makes a directory called ~", fc.store)
	}
	if fc.userAgent == nil || !strings.HasPrefix(*fc.userAgent, "goodread/") {
		t.Errorf("user_agent = %v", fc.userAgent)
	}
}

// TestEveryConfigKeyHasAFlag is the promise in the spec, checked rather than
// asserted. A key that no flag can override is a setting somebody can only
// change by editing a file, which is exactly the ambient behaviour the config
// file is not supposed to introduce.
func TestEveryConfigKeyHasAFlag(t *testing.T) {
	root := NewRootCmd()
	for key, flag := range configKeys {
		if root.PersistentFlags().Lookup(flag) == nil {
			t.Errorf("config key %q maps to --%s, and there is no such flag", key, flag)
		}
	}
}

// TestNoRobotsNotInConfigFile. The override is a decision for one invocation.
// A file that could turn it on would make it ambient, and a month later nobody
// would remember it was set.
func TestNoRobotsNotInConfigFile(t *testing.T) {
	if _, ok := configKeys["no_robots"]; ok {
		t.Fatal("no_robots is a config key")
	}
	p := writeConfig(t, "no_robots = true\n")
	_, err := loadConfigFile(p)
	if err == nil {
		t.Fatal("a config file set no_robots and nothing complained")
	}
	// Refused loudly rather than ignored, so the user does not spend an
	// afternoon working out why the setting does nothing.
	if !strings.Contains(err.Error(), "--no-robots") {
		t.Errorf("the error does not name the flag to use instead: %v", err)
	}
}

func TestConfigFileRefusesWhatItCannotHonour(t *testing.T) {
	for _, body := range []string{
		"pace = \"soon\"\n",
		"retries = \"lots\"\n",
		"no_cache = \"maybe\"\n",
		"nonsense = 1\n",
		"just a line\n",
	} {
		if _, err := loadConfigFile(writeConfig(t, body)); err == nil {
			t.Errorf("%q loaded without complaint, so the run would use settings the user thinks they changed", body)
		}
	}
}

// TestMissingConfigFileIsNotAnError. Most people will never write one.
func TestMissingConfigFileIsNotAnError(t *testing.T) {
	fc, err := loadConfigFile(filepath.Join(t.TempDir(), "nope.toml"))
	if err != nil || fc != nil {
		t.Errorf("loadConfigFile on a missing file = %v, %v", fc, err)
	}
}

// TestFlagBeatsConfigFile. The order is flag, then file, and a run that ignores
// what was typed in favour of a file is a run nobody can help you debug.
func TestFlagBeatsConfigFile(t *testing.T) {
	fc, err := loadConfigFile(writeConfig(t, "pace = \"9s\"\ndepth = \"full\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	// Parsed rather than Set, because Changed is only true on the merged flag
	// set cobra builds while parsing, which is the set applyConfigFile reads.
	root := NewRootCmd()
	if err := root.ParseFlags([]string{"--delay", "5s"}); err != nil {
		t.Fatal(err)
	}
	app := &App{depthArg: "meta"}
	app.cfg.Delay = 5 * time.Second
	app.applyConfigFile(root, fc)

	if app.cfg.Delay != 5*time.Second {
		t.Errorf("delay = %v, and --delay 5s was on the command line", app.cfg.Delay)
	}
	if app.depthArg != "full" {
		t.Errorf("depth = %q, and nothing on the command line set it", app.depthArg)
	}
}
