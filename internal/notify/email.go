package notify

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/reefshepherd/reefshepherd/internal/httpx"
)

const (
	// emailDialTimeout bounds opening the connection to the mail server.
	emailDialTimeout = 10 * time.Second
	// emailTimeout bounds a whole send when the caller sets no deadline.
	emailTimeout = 30 * time.Second
	// emailMaxRecipients is the most addresses one channel mails.
	emailMaxRecipients = 20
	// emailMaxSubject is the longest subject, in characters.
	emailMaxSubject = 200
	// emailMaxCredential is the longest username or password a channel accepts.
	emailMaxCredential = 255
)

// emailHost is what a mail server name may look like: a host name or a literal address.
var emailHost = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

func init() {
	Register(Kind{Name: "email", Secrets: []string{"password"}, Build: emailBuild})
}

type emailConfig struct {
	Host     string   `json:"host"`
	Port     *int     `json:"port"`
	Security string   `json:"security"`
	Username string   `json:"username"`
	Password string   `json:"password"`
	From     string   `json:"from"`
	To       []string `json:"to"`
}

// emailNotifier sends one message through an SMTP server, with the dial, clock and host name as fields so tests can replace them.
type emailNotifier struct {
	host, port, security string
	username, password   string
	from                 *mail.Address
	to                   []*mail.Address

	// tlsBase is the TLS configuration to start from, nil for the system defaults.
	tlsBase  *tls.Config
	dial     func(ctx context.Context, network, address string) (net.Conn, error)
	now      func() time.Time
	hostname func() (string, error)
}

func emailBuild(config json.RawMessage, env Env) (Notifier, error) {
	var cfg emailConfig
	if err := decodeChannelConfig(config, &cfg); err != nil {
		return nil, err
	}
	f := httpx.FieldErrors{Message: configSummary}
	host := emailBareHost(cfg.Host)
	switch {
	case host == "":
		f.Add("config.host", httpx.CodeRequired)
	case len(host) > 253 || (net.ParseIP(host) == nil && !emailHost.MatchString(host)):
		f.Add("config.host", httpx.CodeInvalid)
	}
	switch {
	case cfg.Port == nil:
		f.Add("config.port", httpx.CodeRequired)
	case *cfg.Port < 1 || *cfg.Port > 65535:
		f.Add("config.port", httpx.CodeOutOfRange)
	}
	switch cfg.Security {
	case "":
		f.Add("config.security", httpx.CodeRequired)
	case "starttls", "tls":
	case "none":
		if cfg.Username != "" && !emailIsLoopback(host) {
			f.Add("config.security", httpx.CodeInvalid)
		}
	default:
		f.Add("config.security", httpx.CodeInvalid)
	}
	cleanCredential("config.username", cfg.Username, &f)
	cleanCredential("config.password", cfg.Password, &f)
	if cfg.Username == "" && cfg.Password != "" {
		f.Add("config.username", httpx.CodeRequired)
	}
	if cfg.Username != "" && cfg.Password == "" {
		f.Add("config.password", httpx.CodeRequired)
	}
	from, err := emailAddress(cfg.From)
	if cfg.From == "" {
		f.Add("config.from", httpx.CodeRequired)
	} else if err != nil {
		f.Add("config.from", httpx.CodeInvalid)
	}
	var to []*mail.Address
	switch {
	case len(cfg.To) == 0:
		f.Add("config.to", httpx.CodeRequired)
	case len(cfg.To) > emailMaxRecipients:
		f.Add("config.to", httpx.CodeTooLong)
	default:
		for _, raw := range cfg.To {
			addr, err := emailAddress(raw)
			if err != nil {
				f.Add("config.to", httpx.CodeInvalid)
				break
			}
			to = append(to, addr)
		}
	}
	if err := f.Err(); err != nil {
		return nil, err
	}
	dialer := &net.Dialer{Timeout: emailDialTimeout}
	return &emailNotifier{
		host: host, port: strconv.Itoa(*cfg.Port), security: cfg.Security,
		username: cfg.Username, password: cfg.Password, from: from, to: to,
		dial: dialer.DialContext, now: time.Now, hostname: os.Hostname,
	}, nil
}

// cleanCredential refuses a username or password that is too long or holds a NUL, which the PLAIN login format cannot carry.
func cleanCredential(field, value string, f *httpx.FieldErrors) {
	switch {
	case len(value) > emailMaxCredential:
		f.Add(field, httpx.CodeTooLong)
	case strings.ContainsRune(value, 0):
		f.Add(field, httpx.CodeInvalid)
	}
}

// emailAddress parses one address, optionally with a display name.
func emailAddress(raw string) (*mail.Address, error) {
	if strings.ContainsAny(raw, "\r\n") {
		return nil, errors.New("line break in address")
	}
	return mail.ParseAddress(strings.TrimSpace(raw))
}

// emailBareHost trims a host and the brackets around an IPv6 literal.
func emailBareHost(host string) string {
	return strings.Trim(strings.TrimSpace(host), "[]")
}

// emailIsLoopback reports whether host is "localhost" or a loopback address; a name that merely resolves to one does not count.
func emailIsLoopback(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// emailCheckSecure refuses to hand credentials to a server over an unencrypted connection unless it is on this machine.
func emailCheckSecure(server *smtp.ServerInfo) error {
	if !server.TLS && !emailIsLoopback(server.Name) {
		return errors.New("refusing to send credentials without TLS")
	}
	return nil
}

// emailPlainAuth is the PLAIN login.
type emailPlainAuth struct{ username, password string }

func (a emailPlainAuth) Start(server *smtp.ServerInfo) (string, []byte, error) {
	if err := emailCheckSecure(server); err != nil {
		return "", nil, err
	}
	return "PLAIN", []byte("\x00" + a.username + "\x00" + a.password), nil
}

func (a emailPlainAuth) Next(_ []byte, more bool) ([]byte, error) {
	if more {
		return nil, errors.New("unexpected server challenge")
	}
	return nil, nil
}

// emailLoginAuth is the LOGIN login, which servers such as Microsoft's offer instead of PLAIN.
type emailLoginAuth struct{ username, password string }

func (a emailLoginAuth) Start(server *smtp.ServerInfo) (string, []byte, error) {
	if err := emailCheckSecure(server); err != nil {
		return "", nil, err
	}
	return "LOGIN", nil, nil
}

func (a emailLoginAuth) Next(prompt []byte, more bool) ([]byte, error) {
	if !more {
		return nil, nil
	}
	switch strings.ToLower(strings.TrimSpace(string(prompt))) {
	case "username:", "user name:":
		return []byte(a.username), nil
	case "password:":
		return []byte(a.password), nil
	}
	return nil, errors.New("unexpected server prompt")
}

// auth picks the login the server offers, preferring PLAIN.
func (n *emailNotifier) auth(c *smtp.Client) (smtp.Auth, error) {
	offered, params := c.Extension("AUTH")
	if !offered {
		return nil, errors.New("the server does not offer authentication")
	}
	mechanisms := strings.Fields(strings.ToUpper(params))
	for _, m := range mechanisms {
		if m == "PLAIN" {
			return emailPlainAuth{n.username, n.password}, nil
		}
	}
	for _, m := range mechanisms {
		if m == "LOGIN" {
			return emailLoginAuth{n.username, n.password}, nil
		}
	}
	return nil, errors.New("the server offers no supported login method")
}

// tlsConfig is the TLS configuration for talking to the server, which always verifies its certificate.
func (n *emailNotifier) tlsConfig() *tls.Config {
	cfg := &tls.Config{}
	if n.tlsBase != nil {
		cfg = n.tlsBase.Clone()
	}
	cfg.ServerName = n.host
	cfg.MinVersion = max(cfg.MinVersion, tls.VersionTLS12)
	return cfg
}

// helloName is the name announced to the server: the host name when it is a full one, otherwise this end's address.
func (n *emailNotifier) helloName(conn net.Conn) string {
	if name, err := n.hostname(); err == nil && strings.Contains(name, ".") && emailHost.MatchString(name) {
		return name
	}
	if addr, ok := conn.LocalAddr().(*net.TCPAddr); ok {
		if ip4 := addr.IP.To4(); ip4 != nil {
			return "[" + ip4.String() + "]"
		}
		return "[IPv6:" + addr.IP.String() + "]"
	}
	return "localhost"
}

func (n *emailNotifier) Send(ctx context.Context, msg Message) error {
	data := n.compose(msg)

	deadline := time.Now().Add(emailTimeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	raw, err := n.dial(ctx, "tcp", net.JoinHostPort(n.host, n.port))
	if err != nil {
		return fmt.Errorf("email: connect: %w", err)
	}
	defer raw.Close()
	// Closing the connection is what makes a blocked read return when the send is cancelled.
	stop := context.AfterFunc(ctx, func() { _ = raw.Close() })
	defer stop()
	_ = raw.SetDeadline(deadline)

	conn := raw
	if n.security == "tls" {
		secured := tls.Client(raw, n.tlsConfig())
		if err := secured.HandshakeContext(ctx); err != nil {
			return fmt.Errorf("email: TLS handshake: %w", err)
		}
		conn = secured
	}
	client, err := smtp.NewClient(conn, n.host)
	if err != nil {
		return fmt.Errorf("email: greeting: %w", err)
	}
	defer client.Close()
	return n.transact(client, conn, data)
}

// transact runs the SMTP conversation: greeting, TLS, login, then the message to every recipient.
func (n *emailNotifier) transact(c *smtp.Client, conn net.Conn, data []byte) error {
	if err := c.Hello(n.helloName(conn)); err != nil {
		return fmt.Errorf("email: hello: %w", err)
	}
	if n.security == "starttls" {
		if offered, _ := c.Extension("STARTTLS"); !offered {
			return errors.New("email: the server does not offer STARTTLS")
		}
		if err := c.StartTLS(n.tlsConfig()); err != nil {
			return fmt.Errorf("email: STARTTLS: %w", err)
		}
	}
	if n.username != "" {
		auth, err := n.auth(c)
		if err != nil {
			return fmt.Errorf("email: login: %w", err)
		}
		if err := c.Auth(auth); err != nil {
			return fmt.Errorf("email: login: %w", err)
		}
	}
	if err := c.Mail(n.from.Address); err != nil {
		return fmt.Errorf("email: sender: %w", err)
	}
	for _, rcpt := range n.to {
		if err := c.Rcpt(rcpt.Address); err != nil {
			return fmt.Errorf("email: recipient: %w", err)
		}
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("email: data: %w", err)
	}
	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("email: data: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("email: message not accepted: %w", err)
	}
	// The message is accepted at this point, so a server that drops the goodbye is not a failed send.
	_ = c.Quit()
	return nil
}

// emailStripBreaks removes the characters that would end a header line, so a value cannot add headers of its own.
func emailStripBreaks(s string) string {
	return strings.NewReplacer("\r", "", "\n", "").Replace(s)
}

// messageIDDomain is the part of a Message-ID after the @, taken from the sender's address.
func messageIDDomain(from *mail.Address) string {
	_, domain, ok := strings.Cut(from.Address, "@")
	if !ok || !emailHost.MatchString(domain) {
		return "localhost"
	}
	return domain
}

// emailSubject encodes a title for a Subject header, one encoded word to a line so no line gets long.
func emailSubject(title string) string {
	encoded := mime.QEncoding.Encode("utf-8", emailStripBreaks(clipRunes(title, emailMaxSubject)))
	return strings.ReplaceAll(encoded, "?= =?", "?=\r\n =?")
}

// compose writes the whole message: headers, then the text with the link and the instance name.
func (n *emailNotifier) compose(msg Message) []byte {
	to := make([]string, len(n.to))
	for i, addr := range n.to {
		to[i] = emailStripBreaks(addr.String())
	}
	var b bytes.Buffer
	header := func(name, value string) { b.WriteString(name + ": " + value + "\r\n") }
	header("Date", n.now().Format(time.RFC1123Z))
	header("From", emailStripBreaks(n.from.String()))
	header("To", strings.Join(to, ",\r\n "))
	header("Subject", emailSubject(msg.Title))
	header("Message-ID", "<"+rand.Text()+"@"+messageIDDomain(n.from)+">")
	header("MIME-Version", "1.0")
	header("Content-Type", "text/plain; charset=utf-8")
	header("Content-Transfer-Encoding", "quoted-printable")
	header("Auto-Submitted", "auto-generated")
	b.WriteString("\r\n")

	w := quotedprintable.NewWriter(&b)
	_, _ = w.Write([]byte(emailText(msg)))
	_ = w.Close()
	return b.Bytes()
}

// emailText is the plain-text body: the message, the link when there is one, and a signature naming the instance.
func emailText(msg Message) string {
	var b strings.Builder
	b.WriteString(msg.Body)
	if msg.Link != "" {
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(msg.Link)
	}
	if msg.Instance != "" {
		b.WriteString("\n\n-- \n" + msg.Instance)
	}
	b.WriteString("\n")
	return b.String()
}
