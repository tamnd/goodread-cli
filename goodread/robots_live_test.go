package goodread

import (
	"context"
	"testing"
	"time"
)

// TestNoDisallowedAgainstLiveRules is TestNoDisallowedByDefault against the
// file as it stands today rather than as it stood when it was captured.
//
// The capture is what makes the offline test deterministic, and it is also the
// thing that goes stale. Goodreads can add a Disallow tomorrow and every
// offline test in this package would keep passing while the tool reads a page
// it is no longer allowed to read. So this one fetches.
//
// It skips rather than fails when the site cannot be reached, because a network
// that is down says nothing about this code. It is one request at the default
// pace, and it is skipped under -short.
func TestNoDisallowedAgainstLiveRules(t *testing.T) {
	if testing.Short() {
		t.Skip("fetches robots.txt")
	}
	cfg := DefaultConfig()
	cfg.Retries = 1
	c := NewClient(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	live, err := c.Robots().Get(ctx)
	if err != nil {
		t.Skipf("could not read the live robots.txt, so there is nothing to compare: %v", err)
	}
	if len(live.Rules) == 0 {
		t.Fatal("the live robots.txt parsed to no rules at all")
	}

	for _, o := range Ops {
		if o.Name == "robots" {
			continue // fetching the rules cannot be gated on the rules
		}
		path := SamplePath(o)
		allowed := live.Allowed(path)
		switch {
		case o.Disallowed && allowed:
			t.Errorf("op %q (%s) is marked Disallowed but the live rules permit %s: they relaxed, and the capture is stale",
				o.Name, o.Surface, path)
		case !o.Disallowed && !allowed:
			t.Errorf("op %q (%s) is read by default and the live rules now refuse %s (%v): stop reading it",
				o.Name, o.Surface, path, live.Check(path))
		}
	}

	// The capture is what the offline tests reason about, so a drift between
	// the two is worth saying out loud even when every op still agrees.
	captured := liveRobots(t)
	for _, o := range Ops {
		path := SamplePath(o)
		if captured.Allowed(path) != live.Allowed(path) {
			t.Errorf("testdata/robots.txt disagrees with the live file on %s: recapture it", path)
		}
	}
}
