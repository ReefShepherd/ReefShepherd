package config

import (
	"errors"
	"flag"
	"log/slog"
	"net/netip"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func envOf(pairs map[string]string) LookupFunc {
	return func(key string) (string, bool) {
		v, ok := pairs[key]
		return v, ok
	}
}

func writeFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "reefshepherd.env")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load(nil, envOf(nil))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Listen != ":8484" || cfg.DataDir != "./data" {
		t.Errorf("listen=%q data=%q", cfg.Listen, cfg.DataDir)
	}
	if cfg.LogLevel != slog.LevelInfo || cfg.LogFormat != "text" {
		t.Errorf("level=%v format=%q", cfg.LogLevel, cfg.LogFormat)
	}
	if cfg.AuthMode != "" || cfg.UpdateCheck != nil || cfg.PublicURL != "" || cfg.Dev {
		t.Errorf("unexpected non-zero values: %+v", cfg)
	}
}

func TestLoadPrecedence(t *testing.T) {
	file := writeFile(t, "REEFSHEPHERD_LISTEN=:1111\nREEFSHEPHERD_DATA_DIR=/from/file\nREEFSHEPHERD_LOG_FORMAT=json\n")
	env := envOf(map[string]string{
		EnvListen:  ":2222",
		EnvDataDir: "/from/env",
		EnvConfig:  file,
	})

	t.Run("file only", func(t *testing.T) {
		cfg, err := Load([]string{"--config", file}, envOf(nil))
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Listen != ":1111" || cfg.DataDir != "/from/file" || cfg.LogFormat != "json" {
			t.Errorf("got %+v", cfg)
		}
	})
	t.Run("env beats file", func(t *testing.T) {
		cfg, err := Load(nil, env)
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Listen != ":2222" || cfg.DataDir != "/from/env" {
			t.Errorf("got listen=%q data=%q", cfg.Listen, cfg.DataDir)
		}
		if cfg.LogFormat != "json" {
			t.Errorf("file value not used for unset env: %q", cfg.LogFormat)
		}
		if cfg.ConfigFile != file {
			t.Errorf("config file = %q", cfg.ConfigFile)
		}
	})
	t.Run("flag beats env", func(t *testing.T) {
		cfg, err := Load([]string{"--listen=:3333", "--data-dir", "/from/flag"}, env)
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Listen != ":3333" || cfg.DataDir != "/from/flag" {
			t.Errorf("got listen=%q data=%q", cfg.Listen, cfg.DataDir)
		}
	})
	t.Run("config flag beats config env", func(t *testing.T) {
		other := writeFile(t, "REEFSHEPHERD_LISTEN=:4444\n")
		cfg, err := Load([]string{"--config", other}, envOf(map[string]string{EnvConfig: file}))
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Listen != ":4444" {
			t.Errorf("listen = %q", cfg.Listen)
		}
	})
	t.Run("empty values fall through", func(t *testing.T) {
		cfg, err := Load(nil, envOf(map[string]string{EnvListen: "", EnvConfig: file}))
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Listen != ":1111" {
			t.Errorf("listen = %q", cfg.Listen)
		}
	})
}

func TestLoadDataDirDefaults(t *testing.T) {
	tests := []struct {
		name string
		args []string
		env  map[string]string
		want string
	}{
		{"state directory", nil, map[string]string{"STATE_DIRECTORY": "/var/lib/reefshepherd"}, "/var/lib/reefshepherd"},
		{"first of several state directories", nil, map[string]string{"STATE_DIRECTORY": "/var/lib/a:/var/lib/b"}, "/var/lib/a"},
		{"env beats state directory", nil, map[string]string{"STATE_DIRECTORY": "/s", EnvDataDir: "/e"}, "/e"},
		{"dev default", []string{"--dev"}, nil, "./.devdata"},
		{"dev ignores state directory", []string{"--dev"}, map[string]string{"STATE_DIRECTORY": "/s"}, "./.devdata"},
		{"dev with explicit dir", []string{"--dev", "--data-dir", "/x"}, nil, "/x"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := Load(tt.args, envOf(tt.env))
			if err != nil {
				t.Fatal(err)
			}
			if cfg.DataDir != tt.want {
				t.Errorf("data dir = %q, want %q", cfg.DataDir, tt.want)
			}
		})
	}
}

func TestLoadDevMode(t *testing.T) {
	cfg, err := Load([]string{"--dev"}, envOf(nil))
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Dev || cfg.LogLevel != slog.LevelDebug {
		t.Errorf("dev=%v level=%v", cfg.Dev, cfg.LogLevel)
	}
	if cfg.UpdateCheck == nil || *cfg.UpdateCheck {
		t.Errorf("update check = %v, want false", cfg.UpdateCheck)
	}

	cfg, err = Load([]string{"--dev"}, envOf(map[string]string{EnvLogLevel: "warn", EnvUpdateCheck: "true"}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LogLevel != slog.LevelWarn || cfg.UpdateCheck == nil || !*cfg.UpdateCheck {
		t.Errorf("explicit values lost: level=%v update=%v", cfg.LogLevel, cfg.UpdateCheck)
	}
}

func TestLoadParsedValues(t *testing.T) {
	cfg, err := Load(nil, envOf(map[string]string{
		EnvPublicURL:      "https://reef.home.lan/",
		EnvTrustedProxies: "10.0.0.0/8, 192.168.1.5 ,fd00::/8",
		EnvTrustedOrigins: "https://other.lan, https://reef.home.lan",
		EnvLogLevel:       "WARN",
		EnvLogFormat:      "json",
		EnvAuthMode:       "disabled",
		EnvUpdateCheck:    "false",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PublicURL != "https://reef.home.lan" {
		t.Errorf("public url = %q", cfg.PublicURL)
	}
	wantProxies := []netip.Prefix{
		netip.MustParsePrefix("10.0.0.0/8"),
		netip.MustParsePrefix("192.168.1.5/32"),
		netip.MustParsePrefix("fd00::/8"),
	}
	if !reflect.DeepEqual(cfg.TrustedProxies, wantProxies) {
		t.Errorf("proxies = %v", cfg.TrustedProxies)
	}
	wantOrigins := []string{"https://other.lan", "https://reef.home.lan"}
	if !reflect.DeepEqual(cfg.TrustedOrigins, wantOrigins) {
		t.Errorf("origins = %v (public URL origin must be included once)", cfg.TrustedOrigins)
	}
	if cfg.LogLevel != slog.LevelWarn || cfg.LogFormat != "json" || cfg.AuthMode != "disabled" {
		t.Errorf("got %+v", cfg)
	}
	if cfg.UpdateCheck == nil || *cfg.UpdateCheck {
		t.Errorf("update check = %v", cfg.UpdateCheck)
	}
}

func TestLoadNormalizesOrigins(t *testing.T) {
	tests := []struct {
		name       string
		publicURL  string
		origins    string
		wantPublic string
		want       []string
	}{
		{"mixed case host", "https://Reef.Home.LAN", "", "https://reef.home.lan", []string{"https://reef.home.lan"}},
		{"default https port", "https://reef.lan:443", "", "https://reef.lan", []string{"https://reef.lan"}},
		{"default http port", "HTTP://reef.lan:80", "", "http://reef.lan", []string{"http://reef.lan"}},
		{"non-default port kept", "https://reef.lan:8443", "", "https://reef.lan:8443", []string{"https://reef.lan:8443"}},
		{"port 80 on https kept", "https://reef.lan:80", "", "https://reef.lan:80", []string{"https://reef.lan:80"}},
		{"ipv6 default port", "http://[::1]:80", "", "http://[::1]", []string{"http://[::1]"}},
		{
			"trusted origins and duplicate public url", "https://reef.lan", "HTTPS://Other.LAN:443, http://Plain.lan:80/, https://REEF.lan:443",
			"https://reef.lan", []string{"https://other.lan", "http://plain.lan", "https://reef.lan"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := Load(nil, envOf(map[string]string{EnvPublicURL: tt.publicURL, EnvTrustedOrigins: tt.origins}))
			if err != nil {
				t.Fatal(err)
			}
			if cfg.PublicURL != tt.wantPublic {
				t.Errorf("public url = %q, want %q", cfg.PublicURL, tt.wantPublic)
			}
			if !reflect.DeepEqual(cfg.TrustedOrigins, tt.want) {
				t.Errorf("origins = %v, want %v", cfg.TrustedOrigins, tt.want)
			}
		})
	}
}

func TestLoadErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
		env  map[string]string
		want string
	}{
		{"listen without port", nil, map[string]string{EnvListen: "localhost"}, EnvListen},
		{"listen bad port", nil, map[string]string{EnvListen: ":99999"}, EnvListen},
		{"public url without scheme", nil, map[string]string{EnvPublicURL: "reef.home.lan"}, EnvPublicURL},
		{"public url with path", nil, map[string]string{EnvPublicURL: "https://x.lan/reef"}, "root"},
		{"bad proxy", nil, map[string]string{EnvTrustedProxies: "nope"}, EnvTrustedProxies},
		{"bad origin", nil, map[string]string{EnvTrustedOrigins: "https://x.lan/path"}, EnvTrustedOrigins},
		{"bad log level", nil, map[string]string{EnvLogLevel: "loud"}, EnvLogLevel},
		{"bad log format", nil, map[string]string{EnvLogFormat: "xml"}, EnvLogFormat},
		{"bad auth mode", nil, map[string]string{EnvAuthMode: "oidc"}, EnvAuthMode},
		{"bad update check", nil, map[string]string{EnvUpdateCheck: "maybe"}, EnvUpdateCheck},
		{"unknown flag", []string{"--nope"}, nil, "flag provided but not defined"},
		{"positional argument", []string{"extra"}, nil, "unexpected argument"},
		{"missing config file", []string{"--config", "/does/not/exist.env"}, nil, "open config file"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Load(tt.args, envOf(tt.env))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestLoadConfigFileValidation(t *testing.T) {
	t.Run("unknown REEFSHEPHERD key", func(t *testing.T) {
		file := writeFile(t, "REEFSHEPHERD_LISTN=:1\n")
		_, err := Load([]string{"--config", file}, envOf(nil))
		if err == nil || !strings.Contains(err.Error(), "unknown setting REEFSHEPHERD_LISTN") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("other keys are ignored", func(t *testing.T) {
		file := writeFile(t, "TZ=Europe/Berlin\nREEFSHEPHERD_LISTEN=:1\n")
		cfg, err := Load([]string{"--config", file}, envOf(nil))
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Listen != ":1" {
			t.Errorf("listen = %q", cfg.Listen)
		}
	})
	t.Run("syntax error names file and line", func(t *testing.T) {
		file := writeFile(t, "REEFSHEPHERD_LISTEN=:1\nbroken\n")
		_, err := Load([]string{"--config", file}, envOf(nil))
		if err == nil || !strings.Contains(err.Error(), "line 2") || !strings.Contains(err.Error(), file) {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestLoadHelp(t *testing.T) {
	_, err := Load([]string{"-h"}, envOf(nil))
	if !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("error = %v, want flag.ErrHelp", err)
	}
}

func TestHealthURL(t *testing.T) {
	tests := map[string]string{
		":8484":          "http://127.0.0.1:8484/healthz",
		"0.0.0.0:9000":   "http://127.0.0.1:9000/healthz",
		"[::]:9000":      "http://127.0.0.1:9000/healthz",
		"127.0.0.1:8485": "http://127.0.0.1:8485/healthz",
		"[::1]:8485":     "http://[::1]:8485/healthz",
		"reef.lan:8484":  "http://reef.lan:8484/healthz",
	}
	for listen, want := range tests {
		got, err := Config{Listen: listen}.HealthURL()
		if err != nil || got != want {
			t.Errorf("HealthURL(%q) = %q, %v; want %q", listen, got, err, want)
		}
	}
	if _, err := (Config{Listen: "nonsense"}).HealthURL(); err == nil {
		t.Error("expected error for listen without port")
	}
}
