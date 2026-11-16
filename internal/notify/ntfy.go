package notify

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/reefshepherd/reefshepherd/internal/httpx"
)

const (
	// ntfyDefaultServer is used when a channel names no server.
	ntfyDefaultServer = "https://ntfy.sh"
	// ntfyMaxMessage is the largest message ntfy takes before turning it into an attachment, in bytes.
	ntfyMaxMessage = 4096
	// ntfyMaxTitle is ntfy's title limit in bytes.
	ntfyMaxTitle = 1024
	// ntfyMaxToken is the longest access token a channel accepts.
	ntfyMaxToken = 255
)

// ntfyTopic is what ntfy accepts as a topic name.
var ntfyTopic = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

func init() {
	Register(Kind{Name: "ntfy", Secrets: []string{"topic", "token"}, Build: ntfyBuild})
}

type ntfyConfig struct {
	Server string `json:"server"`
	Topic  string `json:"topic"`
	Token  string `json:"token"`
}

type ntfyNotifier struct {
	env   Env
	root  string
	topic string
	token string
}

func ntfyBuild(config json.RawMessage, env Env) (Notifier, error) {
	var cfg ntfyConfig
	if err := decodeChannelConfig(config, &cfg); err != nil {
		return nil, err
	}
	f := httpx.FieldErrors{Message: configSummary}
	server := strings.TrimSpace(cfg.Server)
	if server == "" {
		server = ntfyDefaultServer
	}
	server = cleanChannelURL(server, "config.server", &f)
	// A query or fragment would end up in the middle of the publish address.
	if strings.ContainsAny(server, "?#") {
		f.Add("config.server", httpx.CodeInvalid)
	}
	switch topic := cfg.Topic; {
	case topic == "":
		f.Add("config.topic", httpx.CodeRequired)
	case !ntfyTopic.MatchString(topic):
		f.Add("config.topic", httpx.CodeInvalid)
	}
	switch token := cfg.Token; {
	case len(token) > ntfyMaxToken:
		f.Add("config.token", httpx.CodeTooLong)
	case strings.ContainsFunc(token, func(r rune) bool { return r <= ' ' || r == 0x7f }):
		f.Add("config.token", httpx.CodeInvalid)
	}
	if err := f.Err(); err != nil {
		return nil, err
	}
	return &ntfyNotifier{env: env, root: strings.TrimRight(server, "/") + "/", topic: cfg.Topic, token: cfg.Token}, nil
}

// ntfyPayload is ntfy's JSON publish format, which avoids the header encoding problems of the plain one.
type ntfyPayload struct {
	Topic    string   `json:"topic"`
	Message  string   `json:"message"`
	Title    string   `json:"title,omitempty"`
	Priority int      `json:"priority"`
	Tags     []string `json:"tags,omitempty"`
	Click    string   `json:"click,omitempty"`
}

func ntfyPriority(severity string) int {
	switch severity {
	case "critical":
		return 5
	case "warning":
		return 4
	}
	return 3
}

func (n *ntfyNotifier) Send(ctx context.Context, msg Message) error {
	payload := ntfyPayload{
		Topic:    n.topic,
		Message:  clipBytes(msg.Body, ntfyMaxMessage),
		Title:    clipBytes(msg.Title, ntfyMaxTitle),
		Priority: ntfyPriority(msg.Severity),
		Click:    msg.Link,
	}
	if msg.EventType != "" {
		payload.Tags = []string{msg.EventType}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("ntfy: encode payload: %w", err)
	}
	var header http.Header
	if n.token != "" {
		header = http.Header{"Authorization": {"Bearer " + n.token}}
	}
	reply, err := postJSON(ctx, n.env, n.root, header, body)
	if err != nil {
		return fmt.Errorf("ntfy: %w", err)
	}
	if reply.ok() {
		return nil
	}
	return replyFailure("ntfy", reply, false, n.root, n.topic, n.token)
}
