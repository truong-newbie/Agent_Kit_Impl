package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"

	"dtkit.local/dtkit/internal/apperr"
	"dtkit.local/dtkit/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	deps := cli.DefaultDeps(os.Stdin, os.Stdout, os.Stderr)
	if err := cli.RunWithDeps(ctx, os.Args[1:], deps); err != nil {
		var rendered *cli.JSONReportedError
		if !errors.As(err, &rendered) {
			fmt.Fprintln(os.Stderr, "dtkit:", err)
		}
		os.Exit(apperr.ExitCodeOf(err))
	}
}
