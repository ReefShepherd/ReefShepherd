package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/reefshepherd/reefshepherd/internal/app"
	"github.com/reefshepherd/reefshepherd/internal/auth"
	"github.com/reefshepherd/reefshepherd/internal/auth/password"
	"github.com/reefshepherd/reefshepherd/internal/config"
	"github.com/reefshepherd/reefshepherd/internal/db"
	"github.com/reefshepherd/reefshepherd/internal/settings"
	"github.com/reefshepherd/reefshepherd/internal/users"
)

// generatedPasswordChars leaves out look-alike characters, since the password is read off a terminal.
const (
	generatedPasswordChars = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789"
	generatedPasswordLen   = 20
)

func adminUsage() string {
	return `Usage: reefshepherd admin <command> [flags]

Commands:
  set-auth-mode <password|disabled>   switch the login mode
  reset-password <username>           set a new random password and sign the user out everywhere
  reset-mfa <username>                remove the user's two-step login
  list-users                          list the accounts

A username goes right after the command; flags such as --data-dir and --config follow it.
The commands open the same database as the running server.
`
}

// adminEnv is everything an admin command works with once the database is open.
type adminEnv struct {
	cfg   config.Config
	db    *db.DB
	store *settings.Store
	auth  *auth.Service
	users *users.Service
}

func runAdmin(ctx context.Context, args []string, env config.LookupFunc, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, adminUsage())
		return 2
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "help", "-h", "--help", "-help":
		fmt.Fprint(stdout, adminUsage())
		return 0
	case "set-auth-mode", "reset-password", "reset-mfa", "list-users":
	default:
		fmt.Fprintf(stderr, "reefshepherd admin: unknown command %q\n\n%s", sub, adminUsage())
		return 2
	}

	var positional string
	wantsArg := sub != "list-users"
	if wantsArg {
		if len(rest) == 0 || len(rest[0]) == 0 || rest[0][0] == '-' {
			fmt.Fprintf(stderr, "reefshepherd admin %s: missing argument\n\n%s", sub, adminUsage())
			return 2
		}
		positional, rest = rest[0], rest[1:]
	}
	cfg, code, ok := loadConfig(rest, env, stderr)
	if !ok {
		return code
	}
	e, err := openAdminEnv(ctx, cfg, app.NewLogger(stderr, cfg.LogLevel, cfg.LogFormat))
	if err != nil {
		fmt.Fprintf(stderr, "reefshepherd admin %s: %v\n", sub, err)
		return 1
	}
	defer e.db.Close()

	switch sub {
	case "set-auth-mode":
		err = e.setAuthMode(ctx, positional, stdout, stderr)
	case "reset-password":
		err = e.resetPassword(ctx, positional, stdout)
	case "reset-mfa":
		err = e.resetMFA(ctx, positional, stdout)
	case "list-users":
		err = e.listUsers(ctx, stdout)
	}
	if err != nil {
		fmt.Fprintf(stderr, "reefshepherd admin %s: %v\n", sub, err)
		return 1
	}
	return 0
}

func openAdminEnv(ctx context.Context, cfg config.Config, logger *slog.Logger) (*adminEnv, error) {
	path := filepath.Join(cfg.DataDir, app.DBFileName)
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("no database at %s (is --data-dir right?): %w", path, err)
	}
	database, err := db.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	// A database from before the first run has no tables, which would otherwise surface as a confusing SQL error.
	if v, err := database.Version(ctx); err != nil || v == 0 {
		_ = database.Close()
		return nil, errors.New("the database has not been set up yet; start the server once first")
	}
	store := settings.NewStore(database)
	authSvc := auth.NewService(auth.Deps{DB: database, Settings: store, Passwords: password.NewHasher(password.DefaultParams()), Config: cfg, Logger: logger})
	return &adminEnv{
		cfg: cfg, db: database, store: store, auth: authSvc,
		users: users.NewService(users.Deps{DB: database, Settings: store, Auth: authSvc, Logger: logger}),
	}, nil
}

func (e *adminEnv) setAuthMode(ctx context.Context, mode string, stdout, stderr io.Writer) error {
	// One transaction, as on the web path, so the acting user and the mode change together even while the server is writing.
	err := e.db.InTx(ctx, func(tx *sql.Tx) error {
		return e.store.WithTx(tx).SwitchAuthMode(ctx, mode)
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Login mode set to %s.\n", mode)
	if e.cfg.AuthMode != "" && e.cfg.AuthMode != mode {
		fmt.Fprintf(stderr, "Note: %s is set to %s and takes precedence until it is removed.\n", config.EnvAuthMode, e.cfg.AuthMode)
	}
	return nil
}

func (e *adminEnv) findUser(ctx context.Context, username string) (id int64, name string, err error) {
	u, err := db.Q(e.db.R).GetUserCredentialsByUsername(ctx, username)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", fmt.Errorf("no user named %q", username)
	}
	if err != nil {
		return 0, "", fmt.Errorf("look up user: %w", err)
	}
	return u.ID, u.Username, nil
}

func (e *adminEnv) resetPassword(ctx context.Context, username string, stdout io.Writer) error {
	id, name, err := e.findUser(ctx, username)
	if err != nil {
		return err
	}
	plain, err := randomPassword()
	if err != nil {
		return err
	}
	hash, err := e.auth.Passwords.Hash(ctx, plain)
	if err != nil {
		return err
	}
	if err := e.auth.SetPassword(ctx, id, hash, 0); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "New password for %s: %s\nAll of their sessions were ended.\n", name, plain)
	return nil
}

func (e *adminEnv) resetMFA(ctx context.Context, username string, stdout io.Writer) error {
	id, name, err := e.findUser(ctx, username)
	if err != nil {
		return err
	}
	if err := e.auth.ResetMFA(ctx, id, 0); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Two-step login removed for %s. All of their sessions were ended.\n", name)
	return nil
}

func (e *adminEnv) listUsers(ctx context.Context, stdout io.Writer) error {
	list, err := e.users.List(ctx)
	if err != nil {
		return err
	}
	tw := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tUSERNAME\tROLE\tDISABLED\tPASSWORD\t2FA\tNAME")
	for _, u := range list {
		fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%s\t%s\t%s\n", u.ID, u.Username, u.Role, yesNo(u.Disabled), yesNo(u.HasPassword), yesNo(u.TOTPEnabled), u.DisplayName)
	}
	return tw.Flush()
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func randomPassword() (string, error) {
	// 256 is not a multiple of the alphabet size, so bytes above the limit are skipped to keep every character equally likely.
	limit := 256 - 256%len(generatedPasswordChars)
	out := make([]byte, 0, generatedPasswordLen)
	buf := make([]byte, 2*generatedPasswordLen)
	for len(out) < generatedPasswordLen {
		if _, err := rand.Read(buf); err != nil {
			return "", fmt.Errorf("generate password: %w", err)
		}
		for _, b := range buf {
			if int(b) < limit && len(out) < generatedPasswordLen {
				out = append(out, generatedPasswordChars[int(b)%len(generatedPasswordChars)])
			}
		}
	}
	return string(out), nil
}
