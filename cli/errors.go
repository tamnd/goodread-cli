package cli

import (
	"context"
	"errors"
	"net"
	"net/url"

	"github.com/tamnd/goodread-cli/goodread"
)

func isBlocked(err error) bool {
	return errors.Is(err, goodread.ErrBlocked) || errors.Is(err, goodread.ErrRateLimited)
}

func isNotFound(err error) bool {
	return errors.Is(err, goodread.ErrNotFound)
}

// isNetwork is the failure where the site never answered.
//
// Worth its own exit code because it is the one failure that is worth
// retrying unchanged. A 404 will still be a 404 in an hour and a disallowed
// path will still be disallowed, but a timeout on a train is just a timeout on
// a train.
func isNetwork(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return true
	}
	var ne net.Error
	if errors.As(err, &ne) {
		return true
	}
	var ue *url.Error
	return errors.As(err, &ue)
}
