package main

import (
	"fmt"
	"os"

	"dtkit.local/dtkit/internal/cli"
)

func main() {
	if err := cli.Run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "dtkit:", err)
		os.Exit(1)
	}
}
