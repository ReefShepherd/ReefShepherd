package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/reefshepherd/reefshepherd/internal/httpx"
)

// webhookCall is one request a webhookServer received.
type webhookCall struct {
	Method, Path, RawQuery string
	Header                 http.Header
	Body                   []byte
}

// webhookReply is what a webhookServer answers with.
type webhookReply struct {
	Status int
	Header map[string]string
	Body   string
}

// webhookServer answers each request with the next reply, repeating the last one, and records the requests.
func webhookServer(t *testing.T, replies ...webhookReply) (*httptest.Server, func() []webhookCall) {
	t.Helper()
	var (
		mu    sync.Mutex
		calls []webhookCall
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		calls = append(calls, webhookCall{Method: r.Method, Path: r.URL.Path, RawQuery: r.URL.RawQuery, Header: r.Header.Clone(), Body: body})
		reply := replies[min(len(calls), len(replies))-1]
		mu.Unlock()
		for k, v := range reply.Header {
			w.Header().Set(k, v)
		}
		w.WriteHeader(reply.Status)
		_, _ = io.WriteString(w, reply.Body)
	}))
	t.Cleanup(srv.Close)
	return srv, func() []webhookCall {
		mu.Lock()
		defer mu.Unlock()
		return append([]webhookCall(nil), calls...)
	}
}

func webhookEnv(srv *httptest.Server) Env {
	return Env{HTTP: srv.Client(), UserAgent: "ReefShepherd/test"}
}

// webhookMessage is the message every provider test sends unless it needs something else.
func webhookMessage() Message {
	return Message{
		EventType: "maintenance_due", Severity: "warning",
		Title: "Water change due", Body: "Tank Reef needs a 20% water change.",
		Link:   "https://reef.example.com/tanks/1/maintenance",
		TankID: 1, TankName: "Reef",
		OccurredAt: time.Date(2026, 11, 15, 9, 0, 0, 0, time.UTC),
		Instance:   "My Reef",
	}
}

// webhookBuilt builds a channel of the given kind from a config and fails the test when it is refused.
func webhookBuilt(t *testing.T, kind, config string, env Env) Notifier {
	t.Helper()
	k, ok := lookup(kind)
	if !ok {
		t.Fatalf("kind %q is not registered", kind)
	}
	n, err := k.Build(json.RawMessage(config), env)
	if err != nil {
		t.Fatalf("build %s %s: %v", kind, config, err)
	}
	return n
}

// webhookRefused builds a config that must be refused and returns the field codes it names.
func webhookRefused(t *testing.T, kind, config string) map[string]string {
	t.Helper()
	k, ok := lookup(kind)
	if !ok {
		t.Fatalf("kind %q is not registered", kind)
	}
	_, err := k.Build(json.RawMessage(config), Env{})
	if err == nil {
		t.Fatalf("%s %s was accepted", kind, config)
	}
	return webhookFieldCodes(t, err)
}

// webhookFieldCodes returns the field codes an error carries.
func webhookFieldCodes(t *testing.T, err error) map[string]string {
	t.Helper()
	f := httpx.FieldErrors{}
	if !f.AddFrom(err) {
		t.Fatalf("error carries no field: %v", err)
	}
	return httpx.Validation(f).Fields
}

// webhookSameJSON fails the test unless got holds the same JSON value as want, where null and an empty list differ.
func webhookSameJSON(t *testing.T, got []byte, want string) {
	t.Helper()
	var g, w any
	if err := json.Unmarshal(got, &g); err != nil {
		t.Fatalf("body is not JSON: %v\n%s", err, got)
	}
	if err := json.Unmarshal([]byte(want), &w); err != nil {
		t.Fatalf("bad expectation: %v", err)
	}
	if !reflect.DeepEqual(g, w) {
		t.Errorf("body\n got: %s\nwant: %s", got, want)
	}
}

func webhookSend(t *testing.T, n Notifier, msg Message) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return n.Send(ctx, msg)
}

func TestWebhookRegistered(t *testing.T) {
	for kind, secrets := range map[string][]string{
		"webhook": {"url"}, "discord": {"url"}, "slack": {"url"}, "ntfy": {"topic", "token"}, "email": {"password"},
	} {
		k, ok := lookup(kind)
		if !ok {
			t.Errorf("kind %s is not registered", kind)
			continue
		}
		if !reflect.DeepEqual(k.Secrets, secrets) {
			t.Errorf("%s secrets = %v, want %v", kind, k.Secrets, secrets)
		}
	}
}

func TestWebhookRequest(t *testing.T) {
	srv, calls := webhookServer(t, webhookReply{Status: 200})
	n := webhookBuilt(t, "webhook", `{"url":"`+srv.URL+`/hook/abc?x=1"}`, webhookEnv(srv))
	if err := webhookSend(t, n, webhookMessage()); err != nil {
		t.Fatal(err)
	}
	got := calls()
	if len(got) != 1 {
		t.Fatalf("%d requests, want 1", len(got))
	}
	c := got[0]
	if c.Method != http.MethodPost || c.Path != "/hook/abc" || c.RawQuery != "x=1" {
		t.Errorf("request = %s %s?%s", c.Method, c.Path, c.RawQuery)
	}
	if c.Header.Get("Content-Type") != "application/json" || c.Header.Get("User-Agent") != "ReefShepherd/test" {
		t.Errorf("headers = %v", c.Header)
	}
	webhookSameJSON(t, c.Body, `{
		"version": 1, "event": "maintenance_due", "severity": "warning",
		"title": "Water change due", "body": "Tank Reef needs a 20% water change.",
		"link": "https://reef.example.com/tanks/1/maintenance",
		"tank": {"id": 1, "name": "Reef"}, "occurred_at": "2026-11-15T09:00:00Z"
	}`)
}

func TestWebhookWithoutTankOrTime(t *testing.T) {
	srv, calls := webhookServer(t, webhookReply{Status: 204})
	n := webhookBuilt(t, "webhook", `{"url":"`+srv.URL+`"}`, webhookEnv(srv))
	msg := Message{EventType: "backup_failed", Severity: "critical", Title: "Backup failed", Body: "disk full"}
	if err := webhookSend(t, n, msg); err != nil {
		t.Fatal(err)
	}
	webhookSameJSON(t, calls()[0].Body, `{
		"version": 1, "event": "backup_failed", "severity": "critical", "title": "Backup failed",
		"body": "disk full", "link": "", "tank": null, "occurred_at": null
	}`)
}

func TestWebhookBuildValidation(t *testing.T) {
	tooLong := `{"url":"https://example.com/` + strings.Repeat("a", 2100) + `"}`
	for name, tc := range map[string]struct {
		config string
		want   map[string]string
	}{
		"missing url":     {`{}`, map[string]string{"config.url": "required"}},
		"blank url":       {`{"url":"  "}`, map[string]string{"config.url": "required"}},
		"not a url":       {`{"url":"not a url"}`, map[string]string{"config.url": "invalid"}},
		"wrong scheme":    {`{"url":"ftp://example.com/x"}`, map[string]string{"config.url": "invalid"}},
		"no host":         {`{"url":"https:///x"}`, map[string]string{"config.url": "invalid"}},
		"too long":        {tooLong, map[string]string{"config.url": "too_long"}},
		"unknown field":   {`{"url":"https://example.com","extra":1}`, map[string]string{"config": "invalid"}},
		"wrong type":      {`{"url":5}`, map[string]string{"config.url": "invalid"}},
		"trailing data":   {`{"url":"https://example.com"} {}`, map[string]string{"config": "invalid"}},
		"not an object":   {`[]`, map[string]string{"config": "invalid"}},
		"truncated json":  {`{"url":`, map[string]string{"config": "invalid"}},
		"nested wrong":    {`{"url":{"a":1}}`, map[string]string{"config.url": "invalid"}},
		"relative url":    {`{"url":"/hook"}`, map[string]string{"config.url": "invalid"}},
		"control chars":   {"{\"url\":\"https://exa\\u0001mple.com\"}", map[string]string{"config.url": "invalid"}},
		"scheme only url": {`{"url":"https://"}`, map[string]string{"config.url": "invalid"}},
	} {
		t.Run(name, func(t *testing.T) {
			if got := webhookRefused(t, "webhook", tc.config); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("fields = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestWebhookBuildAcceptsAnyHost(t *testing.T) {
	for _, u := range []string{"http://localhost:8080/h", "http://192.168.1.20/h", "https://[::1]:9/h", "http://user:pw@nas.lan/h"} {
		webhookBuilt(t, "webhook", `{"url":"`+u+`"}`, Env{})
	}
}

// webhookKinds builds each HTTP kind against srv, for checks that hold for all of them.
func webhookKinds(t *testing.T, srv *httptest.Server) map[string]Notifier {
	t.Helper()
	env := webhookEnv(srv)
	return map[string]Notifier{
		"webhook": webhookBuilt(t, "webhook", `{"url":"`+srv.URL+`/hook/SECRETPATH"}`, env),
		"discord": webhookBuilt(t, "discord", `{"url":"`+srv.URL+`/api/webhooks/1/SECRETPATH"}`, env),
		"slack":   webhookBuilt(t, "slack", `{"url":"`+srv.URL+`/services/T/B/SECRETPATH"}`, env),
		"ntfy":    webhookBuilt(t, "ntfy", `{"server":"`+srv.URL+`","topic":"SECRETPATH"}`, env),
	}
}

func TestWebhookSuccessStatuses(t *testing.T) {
	for _, status := range []int{200, 202, 204} {
		srv, calls := webhookServer(t, webhookReply{Status: status, Body: "ok"})
		for kind, n := range webhookKinds(t, srv) {
			if err := webhookSend(t, n, webhookMessage()); err != nil {
				t.Errorf("%s with %d: %v", kind, status, err)
			}
		}
		if len(calls()) != 4 {
			t.Errorf("%d requests, want 4", len(calls()))
		}
	}
}

func TestWebhookServerErrorIsPlain(t *testing.T) {
	srv, _ := webhookServer(t, webhookReply{Status: 500, Body: "boom"})
	for kind, n := range webhookKinds(t, srv) {
		err := webhookSend(t, n, webhookMessage())
		if err == nil || !strings.Contains(err.Error(), "HTTP 500: boom") || !strings.HasPrefix(err.Error(), kind+": ") {
			t.Errorf("%s: err = %v", kind, err)
		}
		var throttled RetryAfterError
		var gone GoneError
		if errors.As(err, &throttled) || errors.As(err, &gone) {
			t.Errorf("%s: a 500 must be neither throttled nor gone: %v", kind, err)
		}
	}
}

func TestWebhookThrottled(t *testing.T) {
	for name, tc := range map[string]struct {
		reply webhookReply
		want  time.Duration
	}{
		"header seconds":  {webhookReply{Status: 429, Header: map[string]string{"Retry-After": "30"}}, 30 * time.Second},
		"body float":      {webhookReply{Status: 429, Body: `{"message":"You are being rate limited.","retry_after":1.5,"global":false}`}, 1500 * time.Millisecond},
		"body beats head": {webhookReply{Status: 429, Header: map[string]string{"Retry-After": "2"}, Body: `{"retry_after":0.75}`}, 750 * time.Millisecond},
		"no hint":         {webhookReply{Status: 429}, 0},
		"unavailable":     {webhookReply{Status: 503, Header: map[string]string{"Retry-After": "120"}}, 2 * time.Minute},
		"huge is capped":  {webhookReply{Status: 429, Header: map[string]string{"Retry-After": "99999999999"}}, maxRetryAfter},
	} {
		t.Run(name, func(t *testing.T) {
			srv, _ := webhookServer(t, tc.reply)
			for kind, n := range webhookKinds(t, srv) {
				err := webhookSend(t, n, webhookMessage())
				var throttled RetryAfterError
				if !errors.As(err, &throttled) {
					t.Fatalf("%s: err = %v, want a RetryAfterError", kind, err)
				}
				if throttled.RetryAfter() != tc.want {
					t.Errorf("%s: RetryAfter = %v, want %v", kind, throttled.RetryAfter(), tc.want)
				}
			}
		})
	}
}

func TestWebhookRetryAfterDate(t *testing.T) {
	at := time.Now().Add(90 * time.Second).UTC().Format(http.TimeFormat)
	srv, _ := webhookServer(t, webhookReply{Status: 429, Header: map[string]string{"Retry-After": at}})
	err := webhookSend(t, webhookKinds(t, srv)["webhook"], webhookMessage())
	var throttled RetryAfterError
	if !errors.As(err, &throttled) {
		t.Fatalf("err = %v", err)
	}
	if d := throttled.RetryAfter(); d < 80*time.Second || d > 91*time.Second {
		t.Errorf("RetryAfter = %v, want about 90s", d)
	}
}

func TestWebhookUnavailableWithoutHintIsPlain(t *testing.T) {
	srv, _ := webhookServer(t, webhookReply{Status: 503})
	err := webhookSend(t, webhookKinds(t, srv)["webhook"], webhookMessage())
	var throttled RetryAfterError
	if err == nil || errors.As(err, &throttled) {
		t.Errorf("err = %v, want a plain error", err)
	}
}

func TestWebhookErrorDetail(t *testing.T) {
	long := strings.Repeat("é", 400)
	for name, tc := range map[string]struct {
		body string
		want string
	}{
		"discord message":  {`{"message":"Invalid Webhook Token","code":50027}`, "HTTP 400: Invalid Webhook Token"},
		"ntfy error":       {`{"code":40001,"http":400,"error":"topic invalid"}`, "HTTP 400: topic invalid"},
		"plain text":       {"invalid_payload", "HTTP 400: invalid_payload"},
		"multi line":       {"line one\n\n  line\ttwo\r\n", "HTTP 400: line one line two"},
		"control chars":    {"bad\x00\x07news", "HTTP 400: badnews"},
		"no body":          {"", "HTTP 400"},
		"invalid utf8":     {"a\xffb", "HTTP 400: a�b"},
		"json without key": {`{"x":1}`, `HTTP 400: {"x":1}`},
	} {
		t.Run(name, func(t *testing.T) {
			srv, _ := webhookServer(t, webhookReply{Status: 400, Body: tc.body})
			err := webhookSend(t, webhookKinds(t, srv)["webhook"], webhookMessage())
			if err == nil || !strings.HasSuffix(err.Error(), tc.want) {
				t.Errorf("err = %v, want suffix %q", err, tc.want)
			}
		})
	}
	t.Run("truncated by character", func(t *testing.T) {
		srv, _ := webhookServer(t, webhookReply{Status: 400, Body: long})
		err := webhookSend(t, webhookKinds(t, srv)["webhook"], webhookMessage())
		detail := err.Error()[strings.Index(err.Error(), "HTTP 400: ")+len("HTTP 400: "):]
		if utf8.RuneCountInString(detail) != maxReplyDetail || !strings.HasSuffix(detail, ellipsis) || !utf8.ValidString(detail) {
			t.Errorf("detail has %d runes (valid=%v): %q", utf8.RuneCountInString(detail), utf8.ValidString(detail), detail)
		}
	})
}

// webhookRoundTripper fails every request with a fixed error, the way a transport does.
type webhookRoundTripper struct{ err error }

func (r webhookRoundTripper) RoundTrip(*http.Request) (*http.Response, error) { return nil, r.err }

func TestWebhookTransportErrorHidesURL(t *testing.T) {
	const secret = "SECRETPATH"
	cause := &net.DNSError{Err: "no such host", Name: "hooks.private-tunnel.example"}
	env := Env{HTTP: &http.Client{Transport: webhookRoundTripper{cause}}}
	targets := map[string]string{
		"webhook": `{"url":"https://hooks.private-tunnel.example/hook/` + secret + `?t=QUERYSECRET"}`,
		"discord": `{"url":"https://hooks.private-tunnel.example/api/webhooks/1/` + secret + `"}`,
		"slack":   `{"url":"https://hooks.private-tunnel.example/services/T/B/` + secret + `"}`,
		"ntfy":    `{"server":"https://hooks.private-tunnel.example/` + secret + `","topic":"mytopic"}`,
	}
	for kind, config := range targets {
		err := webhookSend(t, webhookBuilt(t, kind, config, env), webhookMessage())
		if err == nil {
			t.Fatalf("%s: no error", kind)
		}
		for _, leak := range []string{secret, "QUERYSECRET", "private-tunnel", "https://", "hooks."} {
			if strings.Contains(err.Error(), leak) {
				t.Errorf("%s: error leaks %q: %v", kind, leak, err)
			}
		}
		if !strings.Contains(err.Error(), "post: ") || !strings.Contains(err.Error(), "no such host") {
			t.Errorf("%s: error lost its operation or cause: %v", kind, err)
		}
		if !errors.Is(err, cause) {
			t.Errorf("%s: the cause is no longer in the error chain", kind)
		}
	}
}

func TestWebhookTransportErrorKeepsContextCause(t *testing.T) {
	srv, _ := webhookServer(t, webhookReply{Status: 200})
	n := webhookKinds(t, srv)["webhook"]
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := n.Send(ctx, webhookMessage())
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled in the chain", err)
	}
	if strings.Contains(err.Error(), srv.URL) || strings.Contains(err.Error(), "SECRETPATH") {
		t.Errorf("error leaks the URL: %v", err)
	}
}

func TestWebhookClosedServerHidesURL(t *testing.T) {
	srv, _ := webhookServer(t, webhookReply{Status: 200})
	n := webhookKinds(t, srv)["webhook"]
	srv.Close()
	err := webhookSend(t, n, webhookMessage())
	if err == nil {
		t.Fatal("no error")
	}
	for _, leak := range []string{"SECRETPATH", "/hook/", "http://", "127.0.0.1"} {
		if strings.Contains(err.Error(), leak) {
			t.Errorf("error leaks %q: %v", leak, err)
		}
	}
}

func TestWebhookRedirectIsAnError(t *testing.T) {
	other, otherCalls := webhookServer(t, webhookReply{Status: 200})
	srv, _ := webhookServer(t, webhookReply{Status: 302, Header: map[string]string{"Location": other.URL + "/elsewhere"}})
	err := webhookSend(t, webhookKinds(t, srv)["webhook"], webhookMessage())
	if err == nil || !strings.Contains(err.Error(), "HTTP 302") {
		t.Errorf("err = %v, want HTTP 302", err)
	}
	if n := len(otherCalls()); n != 0 {
		t.Errorf("the redirect target got %d requests", n)
	}
}

func TestWebhookNilClientStillSends(t *testing.T) {
	srv, calls := webhookServer(t, webhookReply{Status: 200})
	n := webhookBuilt(t, "webhook", `{"url":"`+srv.URL+`"}`, Env{})
	if err := webhookSend(t, n, webhookMessage()); err != nil {
		t.Fatal(err)
	}
	if len(calls()) != 1 || calls()[0].Header.Get("User-Agent") == "" {
		t.Errorf("calls = %+v", calls())
	}
}

func TestWebhookClip(t *testing.T) {
	if got := clipRunes("héllo", 5); got != "héllo" {
		t.Errorf("clipRunes fitting = %q", got)
	}
	got := clipRunes("héllo wörld", 5)
	if got != "héll"+ellipsis || utf8.RuneCountInString(got) != 5 {
		t.Errorf("clipRunes = %q", got)
	}
	if got := clipRunes("a\xffb", 10); got != "a�b" {
		t.Errorf("clipRunes invalid = %q", got)
	}
	for n := 4; n < 12; n++ {
		got := clipBytes(strings.Repeat("é", 20), n)
		if len(got) > n || !utf8.ValidString(got) || !strings.HasSuffix(got, ellipsis) {
			t.Errorf("clipBytes(%d) = %q (%d bytes)", n, got, len(got))
		}
	}
	if got := clipBytes("short", 10); got != "short" {
		t.Errorf("clipBytes fitting = %q", got)
	}
	if !bytes.Equal([]byte(clipBytes("日本語日本語", 10)), []byte("日本"+ellipsis)) {
		t.Errorf("clipBytes cjk = %q", clipBytes("日本語日本語", 10))
	}
}

// webhookNoFragment fails the test when text holds any run of eight characters of a secret, which is what a cut through the middle of one would leave.
func webhookNoFragment(t *testing.T, where, text string, secrets ...string) {
	t.Helper()
	for _, secret := range secrets {
		for i := 0; i+8 <= len(secret); i++ {
			if strings.Contains(text, secret[i:i+8]) {
				t.Errorf("%s leaks part of %q: %s", where, secret, text)
				return
			}
		}
	}
}

// webhookEchoAnswer is a long answer that repeats text after enough padding that cutting it to the detail limit lands inside text.
func webhookEchoAnswer(echo string) string {
	return "<!DOCTYPE html><pre>" + strings.Repeat("p", 130) + " " + echo + " " + strings.Repeat("q", 100) + "</pre>"
}

func TestWebhookAnswerEchoingTheRequestIsMasked(t *testing.T) {
	const token = "AbCdEfGhIjKlMnOpQrStUvWxYz0123456789AbCdEfGhIjKlMnOpQrStUvWxYz01"
	paths := map[string]string{
		"webhook": "/hook/" + token,
		"discord": "/api/webhooks/123456789012345678/" + token,
		"slack":   "/services/T0000/B0000/" + token,
	}
	for kind, path := range paths {
		t.Run(kind, func(t *testing.T) {
			srv, _ := webhookServer(t, webhookReply{Status: 404, Body: webhookEchoAnswer("Cannot POST " + path)})
			n := webhookBuilt(t, kind, `{"url":"`+srv.URL+path+`?key=`+token+`"}`, webhookEnv(srv))
			err := webhookSend(t, n, webhookMessage())
			if err == nil {
				t.Fatal("no error")
			}
			webhookNoFragment(t, kind, err.Error(), token)
			if !strings.Contains(err.Error(), maskedSecret) {
				t.Errorf("the echoed path was not masked: %v", err)
			}
		})
	}
}
