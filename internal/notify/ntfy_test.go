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

// ntfyServer starts a server standing in for an ntfy server and builds the channel that posts to it; config is the part of the channel config after the server.
func ntfyServer(t *testing.T, config string, replies ...webhookReply) (Notifier, func() []webhookCall) {
	t.Helper()
	srv, calls := webhookServer(t, replies...)
	return webhookBuilt(t, "ntfy", `{"server":"`+srv.URL+`"`+config+`}`, webhookEnv(srv)), calls
}

func TestNtfyRequest(t *testing.T) {
	n, calls := ntfyServer(t, `,"topic":"reef_alerts","token":"tk_abc123"`, webhookReply{Status: 200, Body: `{"id":"x","event":"message"}`})
	if err := webhookSend(t, n, webhookMessage()); err != nil {
		t.Fatal(err)
	}
	c := calls()[0]
	if c.Method != http.MethodPost || c.Path != "/" {
		t.Errorf("request = %s %s", c.Method, c.Path)
	}
	if c.Header.Get("Content-Type") != "application/json" || c.Header.Get("User-Agent") != "ReefShepherd/test" || c.Header.Get("Authorization") != "Bearer tk_abc123" {
		t.Errorf("headers = %v", c.Header)
	}
	webhookSameJSON(t, c.Body, `{
		"topic": "reef_alerts",
		"message": "Tank Reef needs a 20% water change.",
		"title": "Water change due",
		"priority": 4,
		"tags": ["maintenance_due"],
		"click": "https://reef.example.com/tanks/1/maintenance"
	}`)
}

func TestNtfyWithoutTokenOrLink(t *testing.T) {
	n, calls := ntfyServer(t, `,"topic":"reef"`, webhookReply{Status: 200})
	msg := webhookMessage()
	msg.Link = ""
	if err := webhookSend(t, n, msg); err != nil {
		t.Fatal(err)
	}
	c := calls()[0]
	if _, ok := c.Header["Authorization"]; ok {
		t.Errorf("an Authorization header was sent without a token: %v", c.Header)
	}
	if strings.Contains(string(c.Body), "click") {
		t.Errorf("body has a click field: %s", c.Body)
	}
}

func TestNtfyServerPathAndDefault(t *testing.T) {
	srv, calls := webhookServer(t, webhookReply{Status: 200})
	n := webhookBuilt(t, "ntfy", `{"server":"`+srv.URL+`/ntfy///","topic":"reef"}`, webhookEnv(srv))
	if err := webhookSend(t, n, webhookMessage()); err != nil {
		t.Fatal(err)
	}
	if got := calls()[0].Path; got != "/ntfy/" {
		t.Errorf("path = %q, want /ntfy/", got)
	}
	built := webhookBuilt(t, "ntfy", `{"topic":"reef"}`, Env{}).(*ntfyNotifier)
	if built.root != "https://ntfy.sh/" {
		t.Errorf("default root = %q", built.root)
	}
}

func TestNtfyPriorities(t *testing.T) {
	for severity, want := range map[string]int{"info": 3, "warning": 4, "critical": 5, "": 3, "weird": 3} {
		n, calls := ntfyServer(t, `,"topic":"reef"`, webhookReply{Status: 200})
		msg := webhookMessage()
		msg.Severity = severity
		if err := webhookSend(t, n, msg); err != nil {
			t.Fatal(err)
		}
		var got struct{ Priority int }
		if err := json.Unmarshal(calls()[0].Body, &got); err != nil || got.Priority != want {
			t.Errorf("severity %q: priority = %d, want %d (%v)", severity, got.Priority, want, err)
		}
	}
}

func TestNtfyTruncatesByBytes(t *testing.T) {
	n, calls := ntfyServer(t, `,"topic":"reef"`, webhookReply{Status: 200})
	msg := webhookMessage()
	msg.Body = strings.Repeat("🐠", 2000)
	msg.Title = strings.Repeat("é", 2000)
	if err := webhookSend(t, n, msg); err != nil {
		t.Fatal(err)
	}
	var got struct{ Message, Title string }
	if err := json.Unmarshal(calls()[0].Body, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Message) > 4096 || !utf8.ValidString(got.Message) || !strings.HasSuffix(got.Message, ellipsis) || len(got.Message) < 4090 {
		t.Errorf("message is %d bytes (valid=%v)", len(got.Message), utf8.ValidString(got.Message))
	}
	if len(got.Title) > 1024 || !utf8.ValidString(got.Title) || !strings.HasSuffix(got.Title, ellipsis) {
		t.Errorf("title is %d bytes (valid=%v)", len(got.Title), utf8.ValidString(got.Title))
	}
	// A message exactly at the limit is sent whole.
	n, calls = ntfyServer(t, `,"topic":"reef"`, webhookReply{Status: 200})
	msg.Body = strings.Repeat("a", 4096)
	if err := webhookSend(t, n, msg); err != nil {
		t.Fatal(err)
	}
	_ = json.Unmarshal(calls()[0].Body, &got)
	if got.Message != msg.Body {
		t.Error("a message at the limit was cut")
	}
}

func TestNtfyFailures(t *testing.T) {
	n, _ := ntfyServer(t, `,"topic":"reef","token":"tk_secret"`, webhookReply{Status: 401, Body: `{"code":40101,"http":401,"error":"unauthorized"}`})
	err := webhookSend(t, n, webhookMessage())
	if err == nil || err.Error() != "ntfy: HTTP 401: unauthorized" {
		t.Errorf("err = %v", err)
	}
	if strings.Contains(err.Error(), "tk_secret") {
		t.Error("the token is in the error")
	}

	n, _ = ntfyServer(t, `,"topic":"reef"`, webhookReply{Status: 429, Header: map[string]string{"Retry-After": "12"}, Body: `{"code":42908,"http":429,"error":"limit reached"}`})
	var throttled RetryAfterError
	if err := webhookSend(t, n, webhookMessage()); !errors.As(err, &throttled) || throttled.RetryAfter().Seconds() != 12 {
		t.Errorf("err = %v", err)
	}

	// A 404 on an ntfy server is a wrong address, which the admin can fix, so the delivery is retried and not given up.
	n, _ = ntfyServer(t, `,"topic":"reef"`, webhookReply{Status: 404, Body: "page not found"})
	var gone GoneError
	if err := webhookSend(t, n, webhookMessage()); err == nil || errors.As(err, &gone) {
		t.Errorf("err = %v, want a plain error", err)
	}
}

func TestNtfyBuildValidation(t *testing.T) {
	for name, tc := range map[string]struct {
		config string
		want   map[string]string
	}{
		"missing topic":    {`{}`, map[string]string{"config.topic": "required"}},
		"topic with slash": {`{"topic":"a/b"}`, map[string]string{"config.topic": "invalid"}},
		"topic with space": {`{"topic":"a b"}`, map[string]string{"config.topic": "invalid"}},
		"topic too long":   {`{"topic":"` + strings.Repeat("a", 65) + `"}`, map[string]string{"config.topic": "invalid"}},
		"bad server":       {`{"server":"ntfy.example.com","topic":"a"}`, map[string]string{"config.server": "invalid"}},
		"ftp server":       {`{"server":"ftp://ntfy.example.com","topic":"a"}`, map[string]string{"config.server": "invalid"}},
		"token with space": {`{"topic":"a","token":"tk abc"}`, map[string]string{"config.token": "invalid"}},
		"token with break": {"{\"topic\":\"a\",\"token\":\"tk\\r\\nX: y\"}", map[string]string{"config.token": "invalid"}},
		"token too long":   {`{"topic":"a","token":"` + strings.Repeat("t", 256) + `"}`, map[string]string{"config.token": "too_long"}},
		"extra field":      {`{"topic":"a","priority":5}`, map[string]string{"config": "invalid"}},
		"server query":     {`{"server":"https://ntfy.example.com/ntfy?a=b","topic":"a"}`, map[string]string{"config.server": "invalid"}},
		"server fragment":  {`{"server":"https://ntfy.example.com/ntfy#top","topic":"a"}`, map[string]string{"config.server": "invalid"}},
		"several":          {`{"server":"x","topic":""}`, map[string]string{"config.server": "invalid", "config.topic": "required"}},
	} {
		t.Run(name, func(t *testing.T) {
			if got := webhookRefused(t, "ntfy", tc.config); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("fields = %v, want %v", got, tc.want)
			}
		})
	}
	webhookBuilt(t, "ntfy", `{"server":"http://192.168.1.5:2586","topic":"a_b-C9","token":"tk_abcdefghijklmnopqrstuvwxyz012"}`, Env{})
}

func TestNtfyAnswerEchoingSecretsIsMasked(t *testing.T) {
	const (
		token = "tk_AbCdEfGhIjKlMnOpQrStUvWxYz0123456789AbCdEfGhIjKlMnOpQrStUvWxYz"
		topic = "TopicSecretAbCdEfGhIjKlMnOpQrStUvWxYz0123456789_abcdefghijklmnop"
		path  = "/private-prefix-AbCdEfGhIjKlMnOpQrStUvWxYz0123456789"
	)
	for name, echo := range map[string]string{
		"token":       "rejected token " + token,
		"topic":       "no such topic " + topic,
		"server path": "Cannot POST " + path + "/",
	} {
		t.Run(name, func(t *testing.T) {
			srv, _ := webhookServer(t, webhookReply{Status: 401, Body: webhookEchoAnswer(echo)})
			n := webhookBuilt(t, "ntfy", `{"server":"`+srv.URL+path+`","topic":"`+topic+`","token":"`+token+`"}`, webhookEnv(srv))
			err := webhookSend(t, n, webhookMessage())
			if err == nil {
				t.Fatal("no error")
			}
			webhookNoFragment(t, name, err.Error(), token, topic, path)
			if !strings.Contains(err.Error(), maskedSecret) {
				t.Errorf("the echo was not masked: %v", err)
			}
		})
	}
}
