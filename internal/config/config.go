// Package config resolves the startup configuration from flags, environment variables and an env-format file.
package config

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Environment variable names, which are also the keys of the config file.
const (
	EnvListen         = "REEFSHEPHERD_LISTEN"
	EnvDataDir        = "REEFSHEPHERD_DATA_DIR"
	EnvConfig         = "REEFSHEPHERD_CONFIG"
	EnvPublicURL      = "REEFSHEPHERD_PUBLIC_URL"
	EnvTrustedProxies = "REEFSHEPHERD_TRUSTED_PROXIES"
	EnvTrustedOrigins = "REEFSHEPHERD_TRUSTED_ORIGINS"
	EnvLogLevel       = "REEFSHEPHERD_LOG_LEVEL"
	EnvLogFormat      = "REEFSHEPHERD_LOG_FORMAT"
	EnvAuthMode       = "REEFSHEPHERD_AUTH_MODE"
	EnvUpdateCheck    = "REEFSHEPHERD_UPDATE_CHECK"

	envStateDirectory = "STATE_DIRECTORY"
	envPrefix         = "REEFSHEPHERD_"

	defaultListen  = ":8484"
	defaultDataDir = "./data"
	devDataDir     = "./.devdata"
)

// Allowed values for the auth mode override.
const (
	AuthModePassword = "password"
	AuthModeDisabled = "disabled"
)

// LookupFunc reads one environment variable, like os.LookupEnv.
type LookupFunc func(key string) (string, bool)

// Config is the startup configuration of the server.
type Config struct {
	// Listen is the address and port to listen on.
	Listen string
	// DataDir holds the database, photos, backups and temporary files.
	DataDir string
	// ConfigFile is the env-format file that was read, empty when none.
	ConfigFile string
	// PublicURL is the external URL without a trailing slash, empty when unset.
	PublicURL string
	// TrustedProxies are the networks whose X-Forwarded-* headers are believed.
	TrustedProxies []netip.Prefix
	// TrustedOrigins are the origins allowed to make cross-origin mutating requests;
	// the origin of PublicURL is always included.
	TrustedOrigins []string
	LogLevel       slog.Level
	// LogFormat is "text" or "json".
	LogFormat string
	// AuthMode is "", "password" or "disabled"; non-empty wins over the web setting.
	AuthMode string
	// UpdateCheck is nil when the web setting decides.
	UpdateCheck *bool
	// Dev enables debug logging, turns the update check off and defaults the data dir to ./.devdata.
	Dev bool
}

// Load resolves the configuration with the precedence flag, environment variable, config file, default, and wraps flag.ErrHelp when -h or -help was given.
func Load(args []string, env LookupFunc) (Config, error) {
	fs := flag.NewFlagSet("reefshepherd", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	listen := fs.String("listen", "", "")
	dataDir := fs.String("data-dir", "", "")
	configFile := fs.String("config", "", "")
	dev := fs.Bool("dev", false, "")
	if err := fs.Parse(args); err != nil {
		return Config{}, fmt.Errorf("parse flags: %w", err)
	}
	if fs.NArg() > 0 {
		return Config{}, fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}

	flagValues := map[string]string{}
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "listen":
			flagValues[EnvListen] = *listen
		case "data-dir":
			flagValues[EnvDataDir] = *dataDir
		case "config":
			flagValues[EnvConfig] = *configFile
		}
	})
	src := source{flags: flagValues, env: env}

	if path := src.get(EnvConfig); path != "" {
		fileValues, err := readEnvFile(path)
		if err != nil {
			return Config{}, err
		}
		src.file = fileValues
	}
	return build(src, *dev)
}

// HealthURL is the URL of the health endpoint as seen from the same machine.
func (c Config) HealthURL() (string, error) {
	host, port, err := net.SplitHostPort(c.Listen)
	if err != nil {
		return "", fmt.Errorf("listen address %q: %w", c.Listen, err)
	}
	if addr, perr := netip.ParseAddr(host); host == "" || (perr == nil && addr.IsUnspecified()) {
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, port) + "/healthz", nil
}

// Usage describes the flags and environment variables.
func Usage() string {
	return `Flags (also accepted by healthcheck):
  --listen ADDR      address and port to listen on (default :8484)
  --data-dir DIR     data directory (default $STATE_DIRECTORY, else ./data)
  --config FILE      env-format config file
  --dev              debug logging, update check off, data dir ./.devdata

Environment variables (and keys of the config file), lower precedence than flags:
  ` + EnvListen + `, ` + EnvDataDir + `, ` + EnvConfig + `, ` + EnvPublicURL + `,
  ` + EnvTrustedProxies + `, ` + EnvTrustedOrigins + `, ` + EnvLogLevel + `,
  ` + EnvLogFormat + `, ` + EnvAuthMode + `, ` + EnvUpdateCheck + `
`
}

func isKnownKey(key string) bool {
	switch key {
	case EnvListen, EnvDataDir, EnvConfig, EnvPublicURL, EnvTrustedProxies,
		EnvTrustedOrigins, EnvLogLevel, EnvLogFormat, EnvAuthMode, EnvUpdateCheck:
		return true
	}
	return false
}

// source looks a key up in flags, then the environment, then the config file; empty values count as unset.
type source struct {
	flags map[string]string
	env   LookupFunc
	file  map[string]string
}

func (s source) get(key string) string {
	if v := strings.TrimSpace(s.flags[key]); v != "" {
		return v
	}
	if s.env != nil {
		if v, ok := s.env(key); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return strings.TrimSpace(s.file[key])
}

func readEnvFile(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open config file: %w", err)
	}
	defer f.Close()
	values, err := ParseEnvFile(f)
	if err != nil {
		return nil, fmt.Errorf("config file %s: %w", path, err)
	}
	for key := range values {
		if strings.HasPrefix(key, envPrefix) && !isKnownKey(key) {
			return nil, fmt.Errorf("config file %s: unknown setting %s", path, key)
		}
	}
	return values, nil
}

func build(src source, dev bool) (Config, error) {
	cfg := Config{Dev: dev, ConfigFile: src.get(EnvConfig)}

	cfg.Listen = orDefault(src.get(EnvListen), defaultListen)
	if err := checkListen(cfg.Listen); err != nil {
		return Config{}, err
	}

	cfg.DataDir = src.get(EnvDataDir)
	if cfg.DataDir == "" {
		cfg.DataDir = defaultDataDirFor(src, dev)
	}

	var err error
	if cfg.PublicURL, err = parsePublicURL(src.get(EnvPublicURL)); err != nil {
		return Config{}, err
	}
	if cfg.TrustedProxies, err = parseProxies(src.get(EnvTrustedProxies)); err != nil {
		return Config{}, err
	}
	if cfg.TrustedOrigins, err = parseOrigins(src.get(EnvTrustedOrigins), cfg.PublicURL); err != nil {
		return Config{}, err
	}

	levelDefault := "info"
	if dev {
		levelDefault = "debug"
	}
	if cfg.LogLevel, err = parseLevel(orDefault(src.get(EnvLogLevel), levelDefault)); err != nil {
		return Config{}, err
	}
	cfg.LogFormat = orDefault(src.get(EnvLogFormat), "text")
	if cfg.LogFormat != "text" && cfg.LogFormat != "json" {
		return Config{}, fmt.Errorf("%s: %q is not text or json", EnvLogFormat, cfg.LogFormat)
	}

	cfg.AuthMode = src.get(EnvAuthMode)
	if cfg.AuthMode != "" && cfg.AuthMode != AuthModePassword && cfg.AuthMode != AuthModeDisabled {
		return Config{}, fmt.Errorf("%s: %q is not %s or %s", EnvAuthMode, cfg.AuthMode, AuthModePassword, AuthModeDisabled)
	}

	if v := src.get(EnvUpdateCheck); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return Config{}, fmt.Errorf("%s: %q is not true or false", EnvUpdateCheck, v)
		}
		cfg.UpdateCheck = &b
	} else if dev {
		off := false
		cfg.UpdateCheck = &off
	}
	return cfg, nil
}

func defaultDataDirFor(src source, dev bool) string {
	if dev {
		return devDataDir
	}
	if src.env != nil {
		if state, ok := src.env(envStateDirectory); ok && state != "" {
			// systemd separates several state directories with a colon.
			first, _, _ := strings.Cut(state, ":")
			return filepath.Clean(first)
		}
	}
	return defaultDataDir
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func checkListen(listen string) error {
	_, port, err := net.SplitHostPort(listen)
	if err != nil {
		return fmt.Errorf("%s: %q is not host:port: %w", EnvListen, listen, err)
	}
	if _, err := strconv.ParseUint(port, 10, 16); err != nil {
		return fmt.Errorf("%s: %q has an invalid port: %w", EnvListen, listen, err)
	}
	return nil
}

func parsePublicURL(raw string) (string, error) {
	if raw == "" {
		return "", nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("%s: %w", EnvPublicURL, err)
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", fmt.Errorf("%s: %q must look like https://host", EnvPublicURL, raw)
	}
	if u.Path != "" && u.Path != "/" {
		return "", fmt.Errorf("%s: %q must not have a path, ReefShepherd is served at the root", EnvPublicURL, raw)
	}
	if u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		return "", fmt.Errorf("%s: %q must be only scheme, host and port", EnvPublicURL, raw)
	}
	return normalizeOrigin(u), nil
}

// normalizeOrigin renders u as scheme://host[:port] the way browsers send it in Origin: lowercase, without a default port.
func normalizeOrigin(u *url.URL) string {
	scheme := strings.ToLower(u.Scheme)
	host := strings.ToLower(u.Hostname())
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	port := u.Port()
	defaultPort := (scheme == "http" && port == "80") || (scheme == "https" && port == "443")
	if port != "" && !defaultPort {
		host += ":" + port
	}
	return scheme + "://" + host
}

func parseProxies(raw string) ([]netip.Prefix, error) {
	var prefixes []netip.Prefix
	for _, item := range splitList(raw) {
		prefix, err := netip.ParsePrefix(item)
		if err != nil {
			addr, aerr := netip.ParseAddr(item)
			if aerr != nil {
				return nil, fmt.Errorf("%s: %q is not a CIDR or IP address", EnvTrustedProxies, item)
			}
			prefix = netip.PrefixFrom(addr, addr.BitLen())
		}
		prefixes = append(prefixes, prefix.Masked())
	}
	return prefixes, nil
}

func parseOrigins(raw, publicURL string) ([]string, error) {
	items := splitList(raw)
	if publicURL != "" {
		items = append(items, publicURL)
	}
	check := http.NewCrossOriginProtection()
	var origins []string
	seen := map[string]bool{}
	for _, item := range items {
		item = strings.TrimRight(item, "/")
		if err := check.AddTrustedOrigin(item); err != nil {
			return nil, fmt.Errorf("%s: %w", EnvTrustedOrigins, err)
		}
		u, err := url.Parse(item)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", EnvTrustedOrigins, err)
		}
		item = normalizeOrigin(u)
		if !seen[item] {
			seen[item] = true
			origins = append(origins, item)
		}
	}
	return origins, nil
}

func parseLevel(s string) (slog.Level, error) {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	}
	return 0, errors.New(EnvLogLevel + ": " + strconv.Quote(s) + " is not debug, info, warn or error")
}

func splitList(raw string) []string {
	var items []string
	for _, item := range strings.Split(raw, ",") {
		if item = strings.TrimSpace(item); item != "" {
			items = append(items, item)
		}
	}
	return items
}
