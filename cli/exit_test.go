package cli

import (
	"context"
	"errors"
	"net"
	"net/url"
	"strings"
	"testing"

	"github.com/tamnd/goodread-cli/goodread"
)

// TestExitCodesMatchTheTable. 05_commands.md section 8 publishes these numbers
// and scripts branch on them, so they are pinned here rather than left to
// whichever constant happens to be declared first.
func TestExitCodesMatchTheTable(t *testing.T) {
	for _, c := range []struct {
		got  int
		want int
		name string
	}{
		{exitError, 1, "unclassified"},
		{exitUsage, 2, "usage and config"},
		{exitNetwork, 3, "network"},
		{exitHTTP, 4, "http error from the site"},
		{exitParse, 5, "parse"},
		{exitNotFound, 6, "not found"},
		{exitDisallowed, 7, "refused by robots"},
		{exitNoRobots, 8, "robots.txt unreadable"},
	} {
		if c.got != c.want {
			t.Errorf("%s = %d, want %d", c.name, c.got, c.want)
		}
	}
}

// TestSevenAndEightStayApart is the distinction the whole flag rests on. 7 is a
// decision the user can reverse by passing --no-robots. 8 is the tool refusing
// to guess because it could not read the rules at all, and no flag turns that
// into a proceed. A script that retries 7 with the override and retries 8 the
// same way would be overriding a rule nobody has read.
func TestSevenAndEightStayApart(t *testing.T) {
	if exitDisallowed == exitNoRobots {
		t.Fatal("7 and 8 are the same code")
	}
	if code(t, goodread.ErrDisallowed) != exitDisallowed {
		t.Error("a disallowed path does not exit 7")
	}
	if code(t, goodread.ErrNoRobots) != exitNoRobots {
		t.Error("an unreadable robots.txt does not exit 8")
	}
	// And the 8 message says nothing was attempted, because a caller who reads
	// only the message has to be able to tell the two apart too.
	err := mapFetchErr(goodread.ErrNoRobots)
	if err == nil || !strings.Contains(err.Error(), "nothing was attempted") {
		t.Errorf("the 8 message does not say nothing was attempted: %v", err)
	}
}

func TestMapFetchErrSortsTheRest(t *testing.T) {
	for _, c := range []struct {
		err  error
		want int
	}{
		{goodread.ErrNotFound, exitNotFound},
		{goodread.ErrBlocked, exitHTTP},
		{goodread.ErrRateLimited, exitHTTP},
		{context.DeadlineExceeded, exitNetwork},
		{&url.Error{Op: "Get", URL: "https://example.com", Err: errors.New("dial")}, exitNetwork},
		{errors.New("something nobody classified"), exitError},
	} {
		if got := code(t, c.err); got != c.want {
			t.Errorf("%v exits %d, want %d", c.err, got, c.want)
		}
	}
	if mapFetchErr(nil) != nil {
		t.Error("no error mapped to an exit code")
	}
}

// TestNetworkIsNotEverything. isNetwork exists to say "try again unchanged", so
// it has to stay narrow. A 404 that got classified as a network blip would have
// scripts retrying a page that will never exist.
func TestNetworkIsNotEverything(t *testing.T) {
	for _, err := range []error{goodread.ErrNotFound, goodread.ErrDisallowed, errors.New("parse failed")} {
		if isNetwork(err) {
			t.Errorf("%v was read as a network failure", err)
		}
	}
	var ne net.Error = &net.DNSError{Err: "no such host", IsTimeout: true}
	if !isNetwork(ne) {
		t.Error("a DNS timeout was not read as a network failure")
	}
}

func code(t *testing.T, err error) int {
	t.Helper()
	var ee *ExitError
	if !errors.As(mapFetchErr(err), &ee) {
		t.Fatalf("%v did not map to an exit code", err)
	}
	return ee.Code
}
