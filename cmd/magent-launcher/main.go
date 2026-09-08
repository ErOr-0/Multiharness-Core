// Command magent-launcher runs Docker on the host; it never executes agents locally.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"multiharness-core/internal/launcher"
)

var version = "dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		fmt.Println("magent Docker launcher", version)
		return
	}
	if err := launcher.Run(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
