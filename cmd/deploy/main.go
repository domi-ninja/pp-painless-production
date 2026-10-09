package main

import (
	"context"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"git.domi.ninja/domi-ninja/infra-meta-forgejo/internal/cli"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	os.Exit(cli.MainContext(ctx, filepath.Base(os.Args[0]), os.Args[1:], os.Stdout, os.Stderr))
}
