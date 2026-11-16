package notify

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/reefshepherd/reefshepherd/internal/testutil"
)

const (
	// The token needs escaping in a URL, and the URL has a secret path, a secret query value and a password.
	redactToken = "s3cret/tok en+x"
	redactURL   = "https://hooker:pa55word-x@hooks.example.com/api/webhooks/123456789/tok%2Fen-part?thread=abc123xyz&x=1"
)

// redactLeaks are the pieces of the channel's secrets that must never reach a client.
var redactLeaks = []string{
	redactToken, url.QueryEscape(redactToken), url.PathEscape(redactToken),
	redactURL, url.QueryEscape(redactURL),
	"/api/webhooks/123456789/tok/en-part", "/api/webhooks/123456789/tok%2Fen-part",
	"tok%2Fen-part", "tok/en-part", "thread=abc123xyz", "abc123xyz", "pa55word-x",
}

// redactErrors are send errors the way Go's HTTP client and providers phrase them, each carrying part of a secret.
var redactErrors = map[string]string{
	"token as is":              "provider rejected token " + redactToken + " (401)",
	"token query escaped":      "provider rejected token " + url.QueryEscape(redactToken) + " (401)",
	"token path escaped":       "provider rejected token " + url.PathEscape(redactToken) + " (401)",
	"url in a client error":    `Post "` + redactURL + `": dial tcp: connection refused`,
	"url escaped":              "relay failed for " + url.QueryEscape(redactURL),
	"decoded path only":        "got 404 for https://hooks.example.com/api/webhooks/123456789/tok/en-part",
	"re-encoded path only":     "got 404 for /api/webhooks/123456789/tok%2Fen-part",
	"one path segment":         "unknown webhook tok%2Fen-part",
	"query only":               "bad request: thread=abc123xyz",
	"query value only":         "no such thread abc123xyz",
	"password only":            "login failed for pa55word-x",
	"several secrets at once":  "token " + redactToken + " at " + redactURL,
	"wrapped in another error": fmt.Sprintf("send: %v", fmt.Errorf("post %q: %w", redactURL, errors.New("EOF"))),
}

func coreAddSecretChannel(t *testing.T, e *coreEnv) Channel {
	t.Helper()
	rec := e.admin(http.MethodPost, channelsPath, map[string]any{
		"name": "secrets", "kind": "fake", "config": map[string]any{"token": redactToken, "url": redactURL, "note": "visible note"},
	})
	testutil.ExpectStatus(t, rec, http.StatusCreated)
	return testutil.Decode[Channel](t, rec)
}

func coreRequireNoLeak(t *testing.T, where, text string) {
	t.Helper()
	for _, leak := range redactLeaks {
		if strings.Contains(text, leak) {
			t.Errorf("%s leaks %q: %s", where, leak, text)
		}
	}
	if !strings.Contains(text, "********") {
		t.Errorf("%s has no mask: %s", where, text)
	}
}

func TestSendErrorsNeverCarryAChannelSecretToTheClient(t *testing.T) {
	for name, text := range redactErrors {
		t.Run(name, func(t *testing.T) {
			e := newCoreEnv(t)
			ch := coreAddSecretChannel(t, e)
			e.raise(coreEvent("a"))
			fakeOutcome(func(context.Context, fakeCall) error { return errors.New(text) })

			// The dispatcher stores the error on the delivery, and the channel list shows it.
			e.dispatch(coreStart)
			d := coreSingle(t, e)
			coreRequireNoLeak(t, "delivery last_error", d.LastError)
			listed := testutil.DecodeItems[Channel](t, e.admin(http.MethodGet, channelsPath, nil))
			coreRequireNoLeak(t, "channel last_error", listed[0].LastError)

			// The test send returns the error to the client directly.
			rec := e.admin(http.MethodPost, channelsPath+"/"+coreID(ch.ID)+"/test", nil)
			testutil.ExpectStatus(t, rec, http.StatusOK)
			got := testutil.Decode[TestResult](t, rec)
			if got.Sent {
				t.Fatal("test send reported success")
			}
			coreRequireNoLeak(t, "test send error", got.Error)
			coreRequireNoLeak(t, "test send body", rec.Body.String())
		})
	}
}

func TestRedactionKeepsTheRestOfTheErrorAndTheNonSecretConfig(t *testing.T) {
	e := newCoreEnv(t)
	coreAddSecretChannel(t, e)
	e.raise(coreEvent("a"))
	fakeOutcome(func(context.Context, fakeCall) error {
		return errors.New("post to hooks.example.com failed: note=visible note, token " + redactToken + ": 401 unauthorized")
	})

	e.dispatch(coreStart)

	want := "post to hooks.example.com failed: note=visible note, token ********: 401 unauthorized"
	if got := coreSingle(t, e).LastError; got != want {
		t.Errorf("last_error = %q, want %q", got, want)
	}
}

func TestRedactionLeavesRetryAfterAndGoneWorking(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status string
		next   time.Duration
	}{
		{"retry after", fmt.Errorf("post %q: %w", redactURL, coreThrottled{5 * time.Minute}), "pending", 5 * time.Minute},
		{"gone", fmt.Errorf("post %q: %w", redactURL, coreGone{}), "failed", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newCoreEnv(t)
			coreAddSecretChannel(t, e)
			e.raise(coreEvent("a"))
			coreFailing(tc.err)

			e.dispatch(coreStart)

			d := coreSingle(t, e)
			coreRequireNoLeak(t, "last_error", d.LastError)
			if d.Status != tc.status {
				t.Fatalf("status = %s, want %s", d.Status, tc.status)
			}
			if tc.next != 0 && (d.NextAttemptAt == nil || *d.NextAttemptAt != coreTime(coreStart.Add(tc.next))) {
				t.Errorf("next attempt = %v, want +%v", d.NextAttemptAt, tc.next)
			}
		})
	}
}

func TestRedactionCoversAConfigThatNoLongerBuilds(t *testing.T) {
	e := newCoreEnv(t)
	// The token is missing, so Build refuses, and the URL secret must still stay out of the message.
	id := testutil.Insert(t, e.db, `INSERT INTO notification_channels (name, kind, config_json) VALUES ('broken', 'fake', ?)`, `{"url":"`+redactURL+`"}`)

	got := testutil.Decode[TestResult](t, e.admin(http.MethodPost, channelsPath+"/"+coreID(id)+"/test", nil))

	if got.Sent || strings.Contains(got.Error, redactURL) || !strings.Contains(got.Error, "config.token") {
		t.Errorf("test = %+v", got)
	}
}
