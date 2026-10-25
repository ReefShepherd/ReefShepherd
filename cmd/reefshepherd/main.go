// Command reefshepherd is the ReefShepherd server and its admin tools.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/reefshepherd/reefshepherd/internal/app"
	"github.com/reefshepherd/reefshepherd/internal/config"
	"github.com/reefshepherd/reefshepherd/internal/version"
)

const healthcheckTimeout = 5 * time.Second

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	// Restore default signal handling after the first signal so a second one ends the process at once.
	go func() {
		<-ctx.Done()
		stop()
	}()
	code := run(ctx, os.Args[1:], os.LookupEnv, os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}

// run executes one subcommand and returns the process exit code.
func run(ctx context.Context, args []string, env config.LookupFunc, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage())
		return 2
	}
	cmd, rest := args[0], args[1:]
	switch cmd {
	case "serve":
		return runServe(ctx, rest, env, stderr)
	case "version":
		fmt.Fprintln(stdout, "reefshepherd", version.String())
		return 0
	case "healthcheck":
		return runHealthcheck(ctx, rest, env, stderr)
	case "admin":
		return runAdmin(ctx, rest, env, stdout, stderr)
	case "restore":
		fmt.Fprintf(stderr, "reefshepherd %s: not implemented yet\n", cmd)
		return 1
	case "help", "-h", "--help", "-help":
		fmt.Fprint(stdout, usage())
		return 0
	}
	fmt.Fprintf(stderr, "reefshepherd: unknown command %q\n\n%s", cmd, usage())
	return 2
}

func usage() string {
	return `Usage: reefshepherd <command> [flags]

Commands:
  serve        run the server
  version      print the version
  healthcheck  exit 0 when the local server answers /healthz
  admin        administration tools
  restore      restore a backup

` + config.Usage()
}

// loadConfig returns ok=false with the exit code to use when the config cannot be loaded or help was asked for.
func loadConfig(args []string, env config.LookupFunc, stderr io.Writer) (cfg config.Config, code int, ok bool) {
	cfg, err := config.Load(args, env)
	if errors.Is(err, flag.ErrHelp) {
		fmt.Fprint(stderr, usage())
		return cfg, 0, false
	}
	if err != nil {
		fmt.Fprintf(stderr, "reefshepherd: %v\n", err)
		return cfg, 2, false
	}
	return cfg, 0, true
}

func runServe(ctx context.Context, args []string, env config.LookupFunc, stderr io.Writer) int {
	cfg, code, ok := loadConfig(args, env, stderr)
	if !ok {
		return code
	}
	logger := app.NewLogger(stderr, cfg.LogLevel, cfg.LogFormat)
	if err := app.Run(ctx, cfg, logger); err != nil {
		logger.Error("server stopped", "error", err)
		return 1
	}
	return 0
}

func runHealthcheck(ctx context.Context, args []string, env config.LookupFunc, stderr io.Writer) int {
	cfg, code, ok := loadConfig(args, env, stderr)
	if !ok {
		return code
	}
	url, err := cfg.HealthURL()
	if err != nil {
		fmt.Fprintf(stderr, "reefshepherd healthcheck: %v\n", err)
		return 1
	}

	ctx, cancel := context.WithTimeout(ctx, healthcheckTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		fmt.Fprintf(stderr, "reefshepherd healthcheck: %v\n", err)
		return 1
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Fprintf(stderr, "reefshepherd healthcheck: %v\n", err)
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(stderr, "reefshepherd healthcheck: %s returned %s\n", url, resp.Status)
		return 1
	}
	return 0
}
