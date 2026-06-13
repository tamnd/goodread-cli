package cli

import (
	"errors"

	"github.com/tamnd/goodread-cli/goodread"
)

func isBlocked(err error) bool {
	return errors.Is(err, goodread.ErrBlocked) || errors.Is(err, goodread.ErrRateLimited)
}

func isNotFound(err error) bool {
	return errors.Is(err, goodread.ErrNotFound)
}
