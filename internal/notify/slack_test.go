package notify

import (
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"
)

// slackServer starts a server standing in for a Slack incoming webhook and builds the channel that posts to it.
func slackServer(t *testing.T, replies ...webhookReply) (Notifier, func() []webhookCall) {
	t.Helper()
	srv, calls := webhookServer(t, replies...)
	return webhookBuilt(t, "slack", `{"url":"`+srv.URL+`/services/T000/B000/XXXX"}`, webhookEnv(srv)), calls
}

func slackSentText(t *testing.T, calls []webhookCall) string {
	t.Helper()
	var body map[string]string
	if err := json.Unmarshal(calls[0].Body, &body); err != nil {
		t.Fatal(err)
	}
	if len(body) != 1 {
		t.Errorf("body has fields besides text: %s", calls[0].Body)
	}
	return body["text"]
}

func TestSlackRequest(t *testing.T) {
	n, calls := slackServer(t, webhookReply{Status: 200, Body: "ok"})
	if err := webhookSend(t, n, webhookMessage()); err != nil {
		t.Fatal(err)
	}
	c := calls()[0]
	if c.Method != http.MethodPost || c.Path != "/services/T000/B000/XXXX" {
		t.Errorf("request = %s %s", c.Method, c.Path)
	}
	if c.Header.Get("Content-Type") != "application/json" || c.Header.Get("User-Agent") != "ReefShepherd/test" {
		t.Errorf("headers = %v", c.Header)
	}
	webhookSameJSON(t, c.Body, `{"text": "*Water change due*\nTank Reef needs a 20% water change.\n<https://reef.example.com/tanks/1/maintenance|Open>"}`)
}

func TestSlackEscapesMarkup(t *testing.T) {
	n, calls := slackServer(t, webhookReply{Status: 200, Body: "ok"})
	msg := webhookMessage()
	msg.Title = "A & B <!channel>"
	msg.Body = "x < y > z && <https://evil.example|click> <@U123>"
	msg.Link = "https://reef.example.com/a b?x=1|2<3>"
	if err := webhookSend(t, n, msg); err != nil {
		t.Fatal(err)
	}
	want := "*A &amp; B &lt;!channel&gt;*\n" +
		"x &lt; y &gt; z &amp;&amp; &lt;https://evil.example|click&gt; &lt;@U123&gt;\n" +
		"<https://reef.example.com/a%20b?x=1%7C2%3C3%3E|Open>"
	if got := slackSentText(t, calls()); got != want {
		t.Errorf("text\n got: %q\nwant: %q", got, want)
	}
}

func TestSlackWithoutBodyOrLink(t *testing.T) {
	n, calls := slackServer(t, webhookReply{Status: 200, Body: "ok"})
	if err := webhookSend(t, n, Message{Title: "Only a title"}); err != nil {
		t.Fatal(err)
	}
	if got := slackSentText(t, calls()); got != "*Only a title*" {
		t.Errorf("text = %q", got)
	}
}

func TestSlackTruncates(t *testing.T) {
	long := func(s string) Message {
		msg := webhookMessage()
		msg.Body = strings.Repeat(s, 6000)
		return msg
	}
	for name, body := range map[string]string{"plain": "é", "entities": "&", "emoji": "🐠"} {
		t.Run(name, func(t *testing.T) {
			n, calls := slackServer(t, webhookReply{Status: 200, Body: "ok"})
			if err := webhookSend(t, n, long(body)); err != nil {
				t.Fatal(err)
			}
			text := slackSentText(t, calls())
			// A whole entity that does not fit is left out, so the text may fall a few characters short.
			if count := utf8.RuneCountInString(text); count > slackMaxText || count < slackMaxText-4 {
				t.Errorf("text has %d characters, want %d", count, slackMaxText)
			}
			if !utf8.ValidString(text) || strings.Contains(text, "�") {
				t.Error("text is not valid UTF-8")
			}
			if !strings.HasSuffix(text, ellipsis+"\n<https://reef.example.com/tanks/1/maintenance|Open>") {
				t.Errorf("the link must survive the cut: %q", text[len(text)-80:])
			}
			// An entity is never split.
			if i := strings.LastIndex(text, "&"); i >= 0 && !strings.HasPrefix(text[i:], "&amp;") {
				t.Errorf("split entity at %q", text[i:i+6])
			}
		})
	}
	n, calls := slackServer(t, webhookReply{Status: 200, Body: "ok"})
	msg := webhookMessage()
	msg.Title = strings.Repeat("T", 2000)
	if err := webhookSend(t, n, msg); err != nil {
		t.Fatal(err)
	}
	text := slackSentText(t, calls())
	if title, _, _ := strings.Cut(text, "\n"); utf8.RuneCountInString(title) != slackMaxTitle+2 {
		t.Errorf("title line has %d characters, want %d", utf8.RuneCountInString(title), slackMaxTitle+2)
	}
}

func TestSlackEscapeLimit(t *testing.T) {
	for _, tc := range []struct {
		in    string
		limit int
		want  string
	}{
		{"a&b", 7, "a&amp;b"},
		{"a&b", 6, "a…"},
		{"a&bcd", 7, "a&amp;…"},
		{"abc", 3, "abc"},
		{"abcd", 3, "ab…"},
		{"a\xffb", 9, "a�b"},
	} {
		if got := slackEscape(tc.in, tc.limit); got != tc.want {
			t.Errorf("slackEscape(%q, %d) = %q, want %q", tc.in, tc.limit, got, tc.want)
		}
	}
}

func TestSlackFailures(t *testing.T) {
	cases := map[string]struct {
		reply    webhookReply
		text     string
		gone     bool
		throttle bool
	}{
		"rate limited":      {webhookReply{Status: 429, Header: map[string]string{"Retry-After": "7"}, Body: "rate_limited"}, "slack: HTTP 429: rate_limited", false, true},
		"bad payload":       {webhookReply{Status: 400, Body: "invalid_payload"}, "slack: HTTP 400: invalid_payload", false, false},
		"no service":        {webhookReply{Status: 403, Body: "no_service"}, "slack: HTTP 403: no_service", true, false},
		"channel not found": {webhookReply{Status: 404, Body: "channel_not_found"}, "slack: HTTP 404: channel_not_found", true, false},
		"unknown hook 404":  {webhookReply{Status: 404, Body: "something"}, "slack: HTTP 404: something", true, false},
		"archived":          {webhookReply{Status: 410, Body: "channel_is_archived"}, "slack: HTTP 410: channel_is_archived", false, false},
		"server error":      {webhookReply{Status: 500, Body: "server_error"}, "slack: HTTP 500: server_error", false, false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			n, _ := slackServer(t, tc.reply)
			err := webhookSend(t, n, webhookMessage())
			if err == nil || err.Error() != tc.text {
				t.Fatalf("err = %v, want %q", err, tc.text)
			}
			var gone GoneError
			if got := errors.As(err, &gone) && gone.Gone(); got != tc.gone {
				t.Errorf("gone = %v, want %v", got, tc.gone)
			}
			var throttled RetryAfterError
			if got := errors.As(err, &throttled); got != tc.throttle {
				t.Errorf("throttled = %v, want %v", got, tc.throttle)
			}
		})
	}
	n, _ := slackServer(t, webhookReply{Status: 429, Header: map[string]string{"Retry-After": "7"}})
	var throttled RetryAfterError
	if err := webhookSend(t, n, webhookMessage()); !errors.As(err, &throttled) || throttled.RetryAfter().Seconds() != 7 {
		t.Errorf("err = %v", err)
	}
}

func TestSlackBuildValidation(t *testing.T) {
	for name, tc := range map[string]struct {
		config string
		want   map[string]string
	}{
		"missing url": {`{}`, map[string]string{"config.url": "required"}},
		"bad url":     {`{"url":"hooks.slack.com/services/x"}`, map[string]string{"config.url": "invalid"}},
		"extra field": {`{"url":"https://hooks.slack.com/services/x","channel":"#a"}`, map[string]string{"config": "invalid"}},
	} {
		t.Run(name, func(t *testing.T) {
			if got := webhookRefused(t, "slack", tc.config); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("fields = %v, want %v", got, tc.want)
			}
		})
	}
}
