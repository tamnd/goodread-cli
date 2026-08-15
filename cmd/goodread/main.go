// Command goodread is a CLI for reading public Goodreads data.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/charmbracelet/fang"
	"github.com/tamnd/goodread-cli/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	root := cli.NewRootCmd()
	err := fang.Execute(ctx, root,
		fang.WithVersion(cli.Version),
		fang.WithNotifySignal(os.Interrupt, syscall.SIGTERM),
		// Errors are printed below, once, verbatim. fang's default handler
		// reflows and title-cases them, which turns a refusal that quotes a
		// robots.txt rule into "/Search is disallowed" on one long line. These
		// messages are written to be read as written.
		fang.WithErrorHandler(func(io.Writer, fang.Styles, error) {}),
	)
	if err == nil {
		return
	}

	var ee *cli.ExitError
	if errors.As(err, &ee) {
		if ee.Err != nil {
			fmt.Fprintln(os.Stderr, "goodread:", ee.Err)
		}
		os.Exit(ee.Code)
	}
	fmt.Fprintln(os.Stderr, "goodread:", err)
	os.Exit(1)
}
