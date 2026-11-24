package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
)

const usage = `Usage:
  frog-cast          open the desktop app
  frog-cast serve    run headless: remote API, media server and casting with no window
  frog-cast --help   show this help

serve listens on the remote API port from the settings file (default 9999)
whether or not the API is enabled there, and on media port 8888. It runs until
SIGINT or SIGTERM. Pages that need a browser engine are refused with the
needs-extraction error, so the phone or desktop extracts them instead.
`

// runCommand handles serve and help; any other command line opens the
// desktop app, as it always has.
func runCommand(args []string, out io.Writer) (handled bool, err error) {
	if len(args) == 0 {
		return false, nil
	}
	switch args[0] {
	case "-h", "--help", "help":
		fmt.Fprint(out, usage)
		return true, nil
	case "serve":
		return true, runHeadless(args[1:], out)
	}
	return false, nil
}

// runHeadless serves remote clients with no window until SIGINT or SIGTERM.
func runHeadless(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(out)
	fs.Usage = func() { fmt.Fprint(out, usage) }
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("serve takes no arguments\n%s", usage)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	app := NewApp()
	if err := app.serveHeadless(ctx); err != nil {
		app.shutdown(context.Background())
		return err
	}
	<-ctx.Done()
	logger.Info("Headless instance stopping")
	app.shutdown(context.Background())
	return nil
}
