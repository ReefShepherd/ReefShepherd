package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func noEnv(string) (string, bool) { return "", false }

func runCmd(args ...string) (code int, stdout, stderr string) {
	var out, errBuf bytes.Buffer
	code = run(context.Background(), args, noEnv, &out, &errBuf)
	return code, out.String(), errBuf.String()
}

func TestVersion(t *testing.T) {
	code, out, _ := runCmd("version")
	if code != 0 || !strings.HasPrefix(out, "reefshepherd ") {
		t.Errorf("code=%d out=%q", code, out)
	}
}

func TestUsageAndUnknownCommand(t *testing.T) {
	if code, _, errOut := runCmd(); code != 2 || !strings.Contains(errOut, "Usage:") {
		t.Errorf("no args: code=%d stderr=%q", code, errOut)
	}
	if code, out, _ := runCmd("help"); code != 0 || !strings.Contains(out, "healthcheck") {
		t.Errorf("help: code=%d out=%q", code, out)
	}
	if code, _, errOut := runCmd("frobnicate"); code != 2 || !strings.Contains(errOut, `unknown command "frobnicate"`) {
		t.Errorf("unknown: code=%d stderr=%q", code, errOut)
	}
}

func TestStubSubcommands(t *testing.T) {
	for _, cmd := range []string{"admin", "restore"} {
		if code, _, errOut := runCmd(cmd); code != 1 || !strings.Contains(errOut, "not implemented") {
			t.Errorf("%s: code=%d stderr=%q", cmd, code, errOut)
		}
	}
}

func TestServeRejectsBadConfig(t *testing.T) {
	code, _, errOut := runCmd("serve", "--listen", "nonsense")
	if code != 2 || !strings.Contains(errOut, "REEFSHEPHERD_LISTEN") {
		t.Errorf("code=%d stderr=%q", code, errOut)
	}
}

func TestHealthcheck(t *testing.T) {
	status := http.StatusOK
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(status)
	}))
	defer srv.Close()
	listen := strings.TrimPrefix(srv.URL, "http://")

	if code, _, errOut := runCmd("healthcheck", "--listen", listen); code != 0 {
		t.Errorf("healthy server: code=%d stderr=%q", code, errOut)
	}

	status = http.StatusServiceUnavailable
	if code, _, errOut := runCmd("healthcheck", "--listen", listen); code != 1 || !strings.Contains(errOut, "503") {
		t.Errorf("unhealthy server: code=%d stderr=%q", code, errOut)
	}

	srv.Close()
	if code, _, _ := runCmd("healthcheck", "--listen", listen); code != 1 {
		t.Errorf("no server: code=%d", code)
	}
}
