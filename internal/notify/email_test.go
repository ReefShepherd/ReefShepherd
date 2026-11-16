package notify

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

// smtpAuthAttempt is a login the fake server received.
type smtpAuthAttempt struct {
	Mechanism, User, Pass string
	Encrypted             bool
}

// smtpSession is what the fake server saw on one connection.
type smtpSession struct {
	Commands []string
	Hello    []string
	StartTLS bool
	Auth     []smtpAuthAttempt
	From     string
	Rcpt     []string
	Data     string
}

// smtpFakeOptions says how the fake server behaves.
type smtpFakeOptions struct {
	ImplicitTLS   bool
	OfferStartTLS bool
	AuthMechs     []string
	User, Pass    string
	RejectRcpt    string
	Silent        bool // accepts connections and never says a word
}

// smtpFake is an in-process SMTP server.
type smtpFake struct {
	opts   smtpFakeOptions
	ln     net.Listener
	cert   tls.Certificate
	roots  *x509.CertPool
	wg     sync.WaitGroup
	mu     sync.Mutex
	sess   []*smtpSession
	conns  []net.Conn
	closed bool
}

// smtpCert makes a certificate for the names the tests use and a pool that trusts it.
func smtpCert(t *testing.T) (tls.Certificate, *x509.CertPool) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "fake smtp"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true, IsCA: true,
		DNSNames: []string{"mail.example.test", "localhost"}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(parsed)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, pool
}

func smtpFakeStart(t *testing.T, opts smtpFakeOptions) *smtpFake {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	f := &smtpFake{opts: opts, ln: ln}
	f.cert, f.roots = smtpCert(t)
	t.Cleanup(func() {
		f.mu.Lock()
		f.closed = true
		for _, c := range f.conns {
			_ = c.Close()
		}
		f.mu.Unlock()
		_ = ln.Close()
		f.wg.Wait()
	})
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			f.mu.Lock()
			if f.closed {
				f.mu.Unlock()
				_ = conn.Close()
				return
			}
			f.conns = append(f.conns, conn)
			f.wg.Add(1)
			f.mu.Unlock()
			go func() {
				defer f.wg.Done()
				f.serve(conn)
			}()
		}
	}()
	return f
}

// dial connects to the fake server whatever address is asked for, so a config can name any host.
func (f *smtpFake) dial(ctx context.Context, _, _ string) (net.Conn, error) {
	var d net.Dialer
	return d.DialContext(ctx, "tcp", f.ln.Addr().String())
}

// sessions waits for every connection to end and returns what they saw.
func (f *smtpFake) sessions() []*smtpSession {
	f.wg.Wait()
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]*smtpSession(nil), f.sess...)
}

func smtpDecode64(s string) string {
	b, _ := base64.StdEncoding.DecodeString(s)
	return string(b)
}

func (f *smtpFake) serve(conn net.Conn) {
	defer conn.Close()
	s := &smtpSession{}
	f.mu.Lock()
	f.sess = append(f.sess, s)
	f.mu.Unlock()
	if f.opts.Silent {
		_, _ = io.Copy(io.Discard, conn)
		return
	}
	encrypted := false
	if f.opts.ImplicitTLS {
		conn = tls.Server(conn, &tls.Config{Certificates: []tls.Certificate{f.cert}})
		encrypted = true
	}
	tp := textproto.NewConn(conn)
	_ = tp.PrintfLine("220 fake ESMTP ready")
	for {
		line, err := tp.ReadLine()
		if err != nil {
			return
		}
		verb, arg, _ := strings.Cut(line, " ")
		verb = strings.ToUpper(verb)
		f.mu.Lock()
		s.Commands = append(s.Commands, verb)
		f.mu.Unlock()
		switch verb {
		case "EHLO", "HELO":
			f.mu.Lock()
			s.Hello = append(s.Hello, arg)
			f.mu.Unlock()
			_ = tp.PrintfLine("250-fake greets %s", arg)
			_ = tp.PrintfLine("250-8BITMIME")
			if f.opts.OfferStartTLS && !encrypted {
				_ = tp.PrintfLine("250-STARTTLS")
			}
			if len(f.opts.AuthMechs) > 0 {
				_ = tp.PrintfLine("250-AUTH %s", strings.Join(f.opts.AuthMechs, " "))
			}
			_ = tp.PrintfLine("250 SIZE 1000000")
		case "STARTTLS":
			if !f.opts.OfferStartTLS || encrypted {
				_ = tp.PrintfLine("502 not offered")
				continue
			}
			_ = tp.PrintfLine("220 go ahead")
			conn = tls.Server(conn, &tls.Config{Certificates: []tls.Certificate{f.cert}})
			tp = textproto.NewConn(conn)
			encrypted = true
			f.mu.Lock()
			s.StartTLS = true
			f.mu.Unlock()
		case "AUTH":
			f.auth(tp, s, arg, encrypted)
		case "MAIL":
			f.mu.Lock()
			s.From = arg
			f.mu.Unlock()
			_ = tp.PrintfLine("250 sender ok")
		case "RCPT":
			if f.opts.RejectRcpt != "" && strings.Contains(arg, f.opts.RejectRcpt) {
				_ = tp.PrintfLine("550 5.1.1 no such user")
				continue
			}
			f.mu.Lock()
			s.Rcpt = append(s.Rcpt, arg)
			f.mu.Unlock()
			_ = tp.PrintfLine("250 recipient ok")
		case "DATA":
			_ = tp.PrintfLine("354 end with a dot")
			var data strings.Builder
			for {
				l, err := tp.R.ReadString('\n')
				if err != nil {
					return
				}
				if l == ".\r\n" {
					break
				}
				// A leading dot was added by the sender to protect the line and is dropped.
				data.WriteString(strings.TrimPrefix(l, "."))
			}
			f.mu.Lock()
			s.Data = data.String()
			f.mu.Unlock()
			_ = tp.PrintfLine("250 queued")
		case "QUIT":
			_ = tp.PrintfLine("221 bye")
			return
		default:
			_ = tp.PrintfLine("250 ok")
		}
	}
}

// auth runs a PLAIN or LOGIN login and records it.
func (f *smtpFake) auth(tp *textproto.Conn, s *smtpSession, arg string, encrypted bool) {
	mech, initial, _ := strings.Cut(arg, " ")
	mech = strings.ToUpper(mech)
	offered := false
	for _, m := range f.opts.AuthMechs {
		offered = offered || strings.EqualFold(m, mech)
	}
	if !offered {
		_ = tp.PrintfLine("504 mechanism not offered")
		return
	}
	attempt := smtpAuthAttempt{Mechanism: mech, Encrypted: encrypted}
	switch mech {
	case "PLAIN":
		if initial == "" {
			_ = tp.PrintfLine("334 ")
			initial, _ = tp.ReadLine()
		}
		parts := strings.Split(smtpDecode64(initial), "\x00")
		if len(parts) == 3 {
			attempt.User, attempt.Pass = parts[1], parts[2]
		}
	case "LOGIN":
		_ = tp.PrintfLine("334 %s", base64.StdEncoding.EncodeToString([]byte("Username:")))
		u, _ := tp.ReadLine()
		_ = tp.PrintfLine("334 %s", base64.StdEncoding.EncodeToString([]byte("Password:")))
		p, _ := tp.ReadLine()
		attempt.User, attempt.Pass = smtpDecode64(u), smtpDecode64(p)
	}
	f.mu.Lock()
	s.Auth = append(s.Auth, attempt)
	f.mu.Unlock()
	if attempt.User == f.opts.User && attempt.Pass == f.opts.Pass {
		_ = tp.PrintfLine("235 2.7.0 welcome")
		return
	}
	_ = tp.PrintfLine("535 5.7.8 credentials not accepted")
}

var smtpFixedNow = time.Date(2026, 11, 15, 10, 30, 0, 0, time.FixedZone("CET", 3600))

// smtpConfig is an email channel config with the given fields replaced; a nil value removes the field.
func smtpConfig(t *testing.T, changes map[string]any) string {
	t.Helper()
	cfg := map[string]any{
		"host": "mail.example.test", "port": 587, "security": "starttls",
		"username": "reef-user", "password": "pw-secret-1",
		"from": "ReefShepherd <reef@example.org>", "to": []string{"a@example.com", "B Name <b@example.com>"},
	}
	for k, v := range changes {
		if v == nil {
			delete(cfg, k)
		} else {
			cfg[k] = v
		}
	}
	b, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// smtpNotifier builds the channel from a config and points it at the fake server, which its certificate is trusted for.
func smtpNotifier(t *testing.T, f *smtpFake, config string) *emailNotifier {
	t.Helper()
	n, ok := webhookBuilt(t, "email", config, Env{}).(*emailNotifier)
	if !ok {
		t.Fatal("not an email notifier")
	}
	n.dial = f.dial
	n.tlsBase = &tls.Config{RootCAs: f.roots}
	n.now = func() time.Time { return smtpFixedNow }
	n.hostname = func() (string, error) { return "reef.lan.example", nil }
	return n
}

// smtpOnly returns the single session a test expects.
func smtpOnly(t *testing.T, f *smtpFake) *smtpSession {
	t.Helper()
	got := f.sessions()
	if len(got) != 1 {
		t.Fatalf("%d sessions, want 1", len(got))
	}
	return got[0]
}

func smtpWorkingServer(opts smtpFakeOptions) smtpFakeOptions {
	if opts.User == "" {
		opts.User, opts.Pass = "reef-user", "pw-secret-1"
	}
	return opts
}

// smtpParse reads the message the server received.
func smtpParse(t *testing.T, s *smtpSession) (*mail.Message, string) {
	t.Helper()
	m, err := mail.ReadMessage(strings.NewReader(s.Data))
	if err != nil {
		t.Fatalf("message does not parse: %v\n%s", err, s.Data)
	}
	body, err := io.ReadAll(quotedprintable.NewReader(m.Body))
	if err != nil {
		t.Fatal(err)
	}
	return m, strings.ReplaceAll(string(body), "\r\n", "\n")
}

func smtpCommands(s *smtpSession) string { return strings.Join(s.Commands, " ") }

func TestEmailStartTLS(t *testing.T) {
	f := smtpFakeStart(t, smtpWorkingServer(smtpFakeOptions{OfferStartTLS: true, AuthMechs: []string{"PLAIN", "LOGIN"}}))
	n := smtpNotifier(t, f, smtpConfig(t, nil))
	if err := webhookSend(t, n, webhookMessage()); err != nil {
		t.Fatal(err)
	}
	s := smtpOnly(t, f)
	if got, want := smtpCommands(s), "EHLO STARTTLS EHLO AUTH MAIL RCPT RCPT DATA QUIT"; got != want {
		t.Errorf("commands = %q, want %q", got, want)
	}
	if !s.StartTLS {
		t.Error("the session was not upgraded")
	}
	if !reflect.DeepEqual(s.Auth, []smtpAuthAttempt{{Mechanism: "PLAIN", User: "reef-user", Pass: "pw-secret-1", Encrypted: true}}) {
		t.Errorf("auth = %+v", s.Auth)
	}
	if !reflect.DeepEqual(s.Hello, []string{"reef.lan.example", "reef.lan.example"}) {
		t.Errorf("hello = %v", s.Hello)
	}
	if s.From != "FROM:<reef@example.org> BODY=8BITMIME" && s.From != "FROM:<reef@example.org>" {
		t.Errorf("mail from = %q", s.From)
	}
	if !reflect.DeepEqual(s.Rcpt, []string{"TO:<a@example.com>", "TO:<b@example.com>"}) {
		t.Errorf("rcpt = %v", s.Rcpt)
	}

	m, body := smtpParse(t, s)
	if d, err := m.Header.Date(); err != nil || !d.Equal(smtpFixedNow) {
		t.Errorf("Date = %v (%v)", d, err)
	}
	if got := m.Header.Get("Date"); got != "Sun, 15 Nov 2026 10:30:00 +0100" {
		t.Errorf("Date header = %q", got)
	}
	if id := m.Header.Get("Message-Id"); !regexp.MustCompile(`^<[A-Z0-9]{10,}@example\.org>$`).MatchString(id) {
		t.Errorf("Message-ID = %q", id)
	}
	if got := m.Header.Get("From"); got != `"ReefShepherd" <reef@example.org>` {
		t.Errorf("From = %q", got)
	}
	if to, err := m.Header.AddressList("To"); err != nil || len(to) != 2 || to[0].Address != "a@example.com" || to[1].Name != "B Name" || to[1].Address != "b@example.com" {
		t.Errorf("To = %v (%v)", to, err)
	}
	if got := m.Header.Get("Subject"); got != "Water change due" {
		t.Errorf("Subject = %q", got)
	}
	for k, v := range map[string]string{
		"Mime-Version": "1.0", "Content-Type": "text/plain; charset=utf-8", "Content-Transfer-Encoding": "quoted-printable",
	} {
		if m.Header.Get(k) != v {
			t.Errorf("%s = %q, want %q", k, m.Header.Get(k), v)
		}
	}
	want := "Tank Reef needs a 20% water change.\n\nhttps://reef.example.com/tanks/1/maintenance\n\n-- \nMy Reef\n"
	if body != want {
		t.Errorf("body = %q, want %q", body, want)
	}
	// Every line ends in CRLF and nothing else.
	if strings.Contains(strings.ReplaceAll(s.Data, "\r\n", ""), "\n") || strings.Contains(strings.ReplaceAll(s.Data, "\r\n", ""), "\r") {
		t.Error("the message has a bare CR or LF")
	}
}

func TestEmailImplicitTLS(t *testing.T) {
	f := smtpFakeStart(t, smtpWorkingServer(smtpFakeOptions{ImplicitTLS: true, AuthMechs: []string{"PLAIN"}}))
	n := smtpNotifier(t, f, smtpConfig(t, map[string]any{"security": "tls", "port": 465}))
	if err := webhookSend(t, n, webhookMessage()); err != nil {
		t.Fatal(err)
	}
	s := smtpOnly(t, f)
	if got, want := smtpCommands(s), "EHLO AUTH MAIL RCPT RCPT DATA QUIT"; got != want {
		t.Errorf("commands = %q, want %q", got, want)
	}
	if len(s.Auth) != 1 || !s.Auth[0].Encrypted || s.Auth[0].Mechanism != "PLAIN" {
		t.Errorf("auth = %+v", s.Auth)
	}
}

func TestEmailNoneOnLocalRelay(t *testing.T) {
	t.Run("no login", func(t *testing.T) {
		f := smtpFakeStart(t, smtpFakeOptions{OfferStartTLS: true})
		n := smtpNotifier(t, f, smtpConfig(t, map[string]any{"host": "relay.lan", "security": "none", "port": 25, "username": nil, "password": nil}))
		if err := webhookSend(t, n, webhookMessage()); err != nil {
			t.Fatal(err)
		}
		s := smtpOnly(t, f)
		// A relay that offers STARTTLS is still spoken to in the clear when the channel says none.
		if got, want := smtpCommands(s), "EHLO MAIL RCPT RCPT DATA QUIT"; got != want {
			t.Errorf("commands = %q, want %q", got, want)
		}
		if len(s.Auth) != 0 || s.StartTLS {
			t.Errorf("auth = %+v, starttls = %v", s.Auth, s.StartTLS)
		}
	})
	for _, host := range []string{"127.0.0.1", "localhost", "::1", "[::1]"} {
		t.Run("login on "+host, func(t *testing.T) {
			f := smtpFakeStart(t, smtpWorkingServer(smtpFakeOptions{AuthMechs: []string{"PLAIN"}}))
			n := smtpNotifier(t, f, smtpConfig(t, map[string]any{"host": host, "security": "none"}))
			if err := webhookSend(t, n, webhookMessage()); err != nil {
				t.Fatal(err)
			}
			s := smtpOnly(t, f)
			if len(s.Auth) != 1 || s.Auth[0].Encrypted || s.Auth[0].User != "reef-user" {
				t.Errorf("auth = %+v", s.Auth)
			}
		})
	}
}

func TestEmailLoginMechanisms(t *testing.T) {
	for name, tc := range map[string]struct {
		mechs []string
		want  string
	}{
		"plain preferred": {[]string{"LOGIN", "PLAIN", "XOAUTH2"}, "PLAIN"},
		"login only":      {[]string{"XOAUTH2", "LOGIN"}, "LOGIN"},
		"lower case":      {[]string{"login"}, "LOGIN"},
	} {
		t.Run(name, func(t *testing.T) {
			f := smtpFakeStart(t, smtpWorkingServer(smtpFakeOptions{OfferStartTLS: true, AuthMechs: tc.mechs}))
			n := smtpNotifier(t, f, smtpConfig(t, nil))
			if err := webhookSend(t, n, webhookMessage()); err != nil {
				t.Fatal(err)
			}
			s := smtpOnly(t, f)
			if len(s.Auth) != 1 || s.Auth[0].Mechanism != tc.want || s.Auth[0].User != "reef-user" || s.Auth[0].Pass != "pw-secret-1" || !s.Auth[0].Encrypted {
				t.Errorf("auth = %+v", s.Auth)
			}
		})
	}
	t.Run("nothing usable", func(t *testing.T) {
		f := smtpFakeStart(t, smtpWorkingServer(smtpFakeOptions{OfferStartTLS: true, AuthMechs: []string{"XOAUTH2", "CRAM-MD5"}}))
		n := smtpNotifier(t, f, smtpConfig(t, nil))
		err := webhookSend(t, n, webhookMessage())
		if err == nil || !strings.Contains(err.Error(), "no supported login method") {
			t.Errorf("err = %v", err)
		}
		if s := smtpOnly(t, f); len(s.Auth) != 0 || strings.Contains(smtpCommands(s), "MAIL") {
			t.Errorf("commands = %v", smtpCommands(s))
		}
	})
	t.Run("no login offered", func(t *testing.T) {
		f := smtpFakeStart(t, smtpFakeOptions{OfferStartTLS: true})
		err := webhookSend(t, smtpNotifier(t, f, smtpConfig(t, nil)), webhookMessage())
		if err == nil || !strings.Contains(err.Error(), "does not offer authentication") {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("wrong password", func(t *testing.T) {
		f := smtpFakeStart(t, smtpFakeOptions{OfferStartTLS: true, AuthMechs: []string{"PLAIN"}, User: "reef-user", Pass: "other"})
		err := webhookSend(t, smtpNotifier(t, f, smtpConfig(t, nil)), webhookMessage())
		if err == nil || !strings.Contains(err.Error(), "535") {
			t.Fatalf("err = %v", err)
		}
		if strings.Contains(err.Error(), "pw-secret-1") || strings.Contains(err.Error(), base64.StdEncoding.EncodeToString([]byte("\x00reef-user\x00pw-secret-1"))) {
			t.Errorf("the password is in the error: %v", err)
		}
	})
}

func TestEmailStartTLSIsMandatory(t *testing.T) {
	f := smtpFakeStart(t, smtpWorkingServer(smtpFakeOptions{AuthMechs: []string{"PLAIN", "LOGIN"}}))
	n := smtpNotifier(t, f, smtpConfig(t, nil))
	err := webhookSend(t, n, webhookMessage())
	if err == nil || !strings.Contains(err.Error(), "does not offer STARTTLS") {
		t.Fatalf("err = %v", err)
	}
	s := smtpOnly(t, f)
	if len(s.Auth) != 0 || s.From != "" || len(s.Rcpt) != 0 || s.Data != "" {
		t.Errorf("the session went on without TLS: %s", smtpCommands(s))
	}
	// Loopback makes no difference: STARTTLS is demanded in this mode whatever the host.
	f = smtpFakeStart(t, smtpWorkingServer(smtpFakeOptions{AuthMechs: []string{"PLAIN"}}))
	n = smtpNotifier(t, f, smtpConfig(t, map[string]any{"host": "127.0.0.1"}))
	if err := webhookSend(t, n, webhookMessage()); err == nil {
		t.Error("a loopback host was allowed to skip STARTTLS")
	}
	if s := smtpOnly(t, f); strings.Contains(smtpCommands(s), "AUTH") || strings.Contains(smtpCommands(s), "MAIL") {
		t.Errorf("commands = %s", smtpCommands(s))
	}
}

func TestEmailNeverSendsCredentialsInTheClear(t *testing.T) {
	// The channel is built by hand to get past the config check and reach the check made when sending.
	f := smtpFakeStart(t, smtpWorkingServer(smtpFakeOptions{AuthMechs: []string{"PLAIN", "LOGIN"}}))
	n := smtpNotifier(t, f, smtpConfig(t, map[string]any{"security": "none", "username": nil, "password": nil}))
	n.username, n.password = "reef-user", "pw-secret-1"
	err := webhookSend(t, n, webhookMessage())
	if err == nil || !strings.Contains(err.Error(), "refusing to send credentials without TLS") {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(err.Error(), "pw-secret-1") {
		t.Errorf("the password is in the error: %v", err)
	}
	s := smtpOnly(t, f)
	if len(s.Auth) != 0 || strings.Contains(smtpCommands(s), "AUTH") || s.Data != "" {
		t.Errorf("credentials or mail went out: %s", smtpCommands(s))
	}

	// The same holds for the LOGIN method.
	f = smtpFakeStart(t, smtpWorkingServer(smtpFakeOptions{AuthMechs: []string{"LOGIN"}}))
	n = smtpNotifier(t, f, smtpConfig(t, map[string]any{"security": "none", "username": nil, "password": nil}))
	n.username, n.password = "reef-user", "pw-secret-1"
	if err := webhookSend(t, n, webhookMessage()); err == nil || len(smtpOnly(t, f).Auth) != 0 {
		t.Errorf("err = %v, auth = %+v", err, f.sessions()[0].Auth)
	}

	// The login methods themselves refuse a clear connection to a remote host and accept one to this machine.
	for name, auth := range map[string]smtp.Auth{"PLAIN": emailPlainAuth{"u", "p"}, "LOGIN": emailLoginAuth{"u", "p"}} {
		for host, tc := range map[string]struct {
			tls     bool
			allowed bool
		}{"mail.example.com": {false, false}, "mail.example.com ": {true, true}, "localhost": {false, true}, "127.0.0.1": {false, true}, "192.168.1.5": {false, false}} {
			_, _, err := auth.Start(&smtp.ServerInfo{Name: strings.TrimSpace(host), TLS: tc.tls})
			if (err == nil) != tc.allowed {
				t.Errorf("%s to %q (tls=%v): err = %v, want allowed=%v", name, host, tc.tls, err, tc.allowed)
			}
		}
	}
}

func TestEmailVerifiesTheCertificate(t *testing.T) {
	f := smtpFakeStart(t, smtpWorkingServer(smtpFakeOptions{OfferStartTLS: true, AuthMechs: []string{"PLAIN"}}))
	n := smtpNotifier(t, f, smtpConfig(t, nil))
	n.tlsBase = nil
	err := webhookSend(t, n, webhookMessage())
	if err == nil || !strings.Contains(err.Error(), "STARTTLS") || !strings.Contains(err.Error(), "certificate") {
		t.Fatalf("err = %v", err)
	}
	if s := smtpOnly(t, f); len(s.Auth) != 0 || s.Data != "" {
		t.Errorf("the session went on: %s", smtpCommands(s))
	}

	f = smtpFakeStart(t, smtpWorkingServer(smtpFakeOptions{ImplicitTLS: true, AuthMechs: []string{"PLAIN"}}))
	n = smtpNotifier(t, f, smtpConfig(t, map[string]any{"security": "tls"}))
	n.host = "other.example.test"
	if err := webhookSend(t, n, webhookMessage()); err == nil || !strings.Contains(err.Error(), "TLS handshake") {
		t.Errorf("a certificate for another name was accepted: %v", err)
	}
}

func TestEmailHelloName(t *testing.T) {
	f := smtpFakeStart(t, smtpFakeOptions{})
	n := smtpNotifier(t, f, smtpConfig(t, map[string]any{"security": "none", "username": nil, "password": nil}))
	n.hostname = func() (string, error) { return "reef", nil }
	if err := webhookSend(t, n, webhookMessage()); err != nil {
		t.Fatal(err)
	}
	if got := smtpOnly(t, f).Hello; !reflect.DeepEqual(got, []string{"[127.0.0.1]"}) {
		t.Errorf("a short host name should give the address, got %v", got)
	}
	f = smtpFakeStart(t, smtpFakeOptions{})
	n = smtpNotifier(t, f, smtpConfig(t, map[string]any{"security": "none", "username": nil, "password": nil}))
	n.hostname = func() (string, error) { return "", fmt.Errorf("no name") }
	if err := webhookSend(t, n, webhookMessage()); err != nil {
		t.Fatal(err)
	}
	if got := smtpOnly(t, f).Hello; !reflect.DeepEqual(got, []string{"[127.0.0.1]"}) {
		t.Errorf("hello = %v", got)
	}
}

func TestEmailHeadersCannotBeInjected(t *testing.T) {
	f := smtpFakeStart(t, smtpWorkingServer(smtpFakeOptions{OfferStartTLS: true, AuthMechs: []string{"PLAIN"}}))
	n := smtpNotifier(t, f, smtpConfig(t, nil))
	// Names that Build would refuse are put in by hand to prove the headers are cleaned as well.
	n.from = &mail.Address{Name: "Evil\r\nBcc: from@example.net", Address: "reef@example.org"}
	n.to = []*mail.Address{{Name: "Eve\r\nBcc: to@example.net", Address: "a@example.com"}}
	msg := webhookMessage()
	msg.Title = "Hi\r\nBcc: attacker@example.net\r\n\r\nInjected body"
	msg.Body = "line one\r\nline two"
	if err := webhookSend(t, n, msg); err != nil {
		t.Fatal(err)
	}
	s := smtpOnly(t, f)
	m, body := smtpParse(t, s)
	for key := range m.Header {
		switch key {
		case "Date", "From", "To", "Subject", "Message-Id", "Mime-Version", "Content-Type", "Content-Transfer-Encoding", "Auto-Submitted":
		default:
			t.Errorf("unexpected header %q", key)
		}
	}
	if got := m.Header.Get("Subject"); got != "HiBcc: attacker@example.netInjected body" {
		t.Errorf("Subject = %q", got)
	}
	if !strings.HasPrefix(body, "line one\nline two\n") {
		t.Errorf("body = %q", body)
	}
	head, _, _ := strings.Cut(s.Data, "\r\n\r\n")
	if strings.Contains(head, "\r\nBcc:") || strings.Contains(head, "\r\nInjected") {
		t.Errorf("a header was injected:\n%s", head)
	}
	if len(s.Rcpt) != 1 {
		t.Errorf("rcpt = %v", s.Rcpt)
	}
}

func TestEmailEncodesNonASCII(t *testing.T) {
	f := smtpFakeStart(t, smtpWorkingServer(smtpFakeOptions{OfferStartTLS: true, AuthMechs: []string{"PLAIN"}}))
	n := smtpNotifier(t, f, smtpConfig(t, map[string]any{"from": "Riff Wächter <reef@example.org>"}))
	msg := webhookMessage()
	msg.Title = strings.Repeat("Wasserwechsel fällig 🐠 ", 12)
	msg.Body = strings.Repeat("Süßwasser ", 300) + "\n.\n.dot first\n" + strings.Repeat("x", 1200)
	if err := webhookSend(t, n, msg); err != nil {
		t.Fatal(err)
	}
	s := smtpOnly(t, f)
	head, text, _ := strings.Cut(s.Data, "\r\n\r\n")
	for i, line := range strings.Split(text, "\r\n") {
		if len(line) > 78 {
			t.Errorf("body line %d is %d bytes: %q", i, len(line), line)
		}
	}
	for i, line := range strings.Split(head, "\r\n") {
		if len(line) > 90 {
			t.Errorf("header line %d is %d bytes: %q", i, len(line), line)
		}
	}
	for _, line := range strings.Split(head, "\r\n") {
		for _, r := range line {
			if r > 127 {
				t.Errorf("header has a non-ASCII character: %q", line)
			}
		}
	}
	m, body := smtpParse(t, s)
	subject, err := new(mime.WordDecoder).DecodeHeader(m.Header.Get("Subject"))
	if want := clipRunes(msg.Title, emailMaxSubject); err != nil || subject != want {
		t.Errorf("Subject = %q (%v)", subject, err)
	}
	if !strings.HasPrefix(m.Header.Get("Subject"), "=?utf-8?q?") {
		t.Errorf("Subject is not encoded: %q", m.Header.Get("Subject"))
	}
	from, _ := m.Header.AddressList("From")
	if len(from) != 1 || from[0].Name != "Riff Wächter" {
		t.Errorf("From = %v", from)
	}
	if !strings.HasPrefix(body, msg.Body) {
		t.Errorf("body does not round trip: %q", body[:80])
	}
}

func TestEmailLongSubjectIsCut(t *testing.T) {
	f := smtpFakeStart(t, smtpWorkingServer(smtpFakeOptions{OfferStartTLS: true, AuthMechs: []string{"PLAIN"}}))
	n := smtpNotifier(t, f, smtpConfig(t, nil))
	msg := webhookMessage()
	msg.Title = strings.Repeat("a", 500)
	if err := webhookSend(t, n, msg); err != nil {
		t.Fatal(err)
	}
	m, _ := smtpParse(t, smtpOnly(t, f))
	got, err := new(mime.WordDecoder).DecodeHeader(m.Header.Get("Subject"))
	if want := strings.Repeat("a", emailMaxSubject-1) + ellipsis; err != nil || got != want {
		t.Errorf("Subject has %d characters: %q (%v)", len([]rune(got)), got, err)
	}
}

func TestEmailRecipientRefused(t *testing.T) {
	f := smtpFakeStart(t, smtpWorkingServer(smtpFakeOptions{OfferStartTLS: true, AuthMechs: []string{"PLAIN"}, RejectRcpt: "b@example.com"}))
	err := webhookSend(t, smtpNotifier(t, f, smtpConfig(t, nil)), webhookMessage())
	if err == nil || !strings.Contains(err.Error(), "recipient") || !strings.Contains(err.Error(), "550") {
		t.Errorf("err = %v", err)
	}
	if s := smtpOnly(t, f); s.Data != "" {
		t.Error("the message was sent although a recipient was refused")
	}
}

func TestEmailTimesOut(t *testing.T) {
	f := smtpFakeStart(t, smtpFakeOptions{Silent: true})
	n := smtpNotifier(t, f, smtpConfig(t, nil))
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	if err := n.Send(ctx, webhookMessage()); err == nil {
		t.Error("a server that never answers was not an error")
	}
	if took := time.Since(start); took > 3*time.Second {
		t.Errorf("took %v", took)
	}
}

func TestEmailCancelStopsASend(t *testing.T) {
	f := smtpFakeStart(t, smtpFakeOptions{Silent: true})
	n := smtpNotifier(t, f, smtpConfig(t, nil))
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(100*time.Millisecond, cancel)
	done := make(chan error, 1)
	go func() { done <- n.Send(ctx, webhookMessage()) }()
	select {
	case err := <-done:
		if err == nil {
			t.Error("no error after cancel")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the send did not stop when cancelled")
	}
}

func TestEmailUnreachableServer(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	n := webhookBuilt(t, "email", smtpConfig(t, map[string]any{"security": "none", "username": nil, "password": nil}), Env{}).(*emailNotifier)
	n.dial = func(ctx context.Context, network, _ string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, network, addr)
	}
	err = webhookSend(t, n, webhookMessage())
	if err == nil || !strings.HasPrefix(err.Error(), "email: connect: ") {
		t.Errorf("err = %v", err)
	}
}

func TestEmailBuildValidation(t *testing.T) {
	for name, tc := range map[string]struct {
		changes map[string]any
		want    map[string]string
	}{
		"no host":                   {map[string]any{"host": nil}, map[string]string{"config.host": "required"}},
		"blank host":                {map[string]any{"host": " "}, map[string]string{"config.host": "required"}},
		"host with slash":           {map[string]any{"host": "mail.example.com/x"}, map[string]string{"config.host": "invalid"}},
		"host with scheme":          {map[string]any{"host": "smtp://mail.example.com"}, map[string]string{"config.host": "invalid"}},
		"host with port":            {map[string]any{"host": "mail.example.com:587"}, map[string]string{"config.host": "invalid"}},
		"host with space":           {map[string]any{"host": "mail example.com"}, map[string]string{"config.host": "invalid"}},
		"no port":                   {map[string]any{"port": nil}, map[string]string{"config.port": "required"}},
		"port zero":                 {map[string]any{"port": 0}, map[string]string{"config.port": "out_of_range"}},
		"port too big":              {map[string]any{"port": 65536}, map[string]string{"config.port": "out_of_range"}},
		"port negative":             {map[string]any{"port": -1}, map[string]string{"config.port": "out_of_range"}},
		"port as text":              {map[string]any{"port": "587"}, map[string]string{"config.port": "invalid"}},
		"port fraction":             {map[string]any{"port": 587.5}, map[string]string{"config.port": "invalid"}},
		"no security":               {map[string]any{"security": nil}, map[string]string{"config.security": "required"}},
		"unknown security":          {map[string]any{"security": "ssl"}, map[string]string{"config.security": "invalid"}},
		"login with none remote":    {map[string]any{"security": "none"}, map[string]string{"config.security": "invalid"}},
		"password alone":            {map[string]any{"username": nil}, map[string]string{"config.username": "required"}},
		"username alone":            {map[string]any{"password": nil}, map[string]string{"config.password": "required"}},
		"username too long":         {map[string]any{"username": strings.Repeat("u", 256)}, map[string]string{"config.username": "too_long"}},
		"password with NUL":         {map[string]any{"password": "a\x00b"}, map[string]string{"config.password": "invalid"}},
		"no from":                   {map[string]any{"from": nil}, map[string]string{"config.from": "required"}},
		"bad from":                  {map[string]any{"from": "not an address"}, map[string]string{"config.from": "invalid"}},
		"from with line break":      {map[string]any{"from": "Reef <a@example.org>\r\nBcc: x@example.net"}, map[string]string{"config.from": "invalid"}},
		"from name with line break": {map[string]any{"from": "Reef\r\nBcc: x@example.net <a@example.org>"}, map[string]string{"config.from": "invalid"}},
		"no recipients":             {map[string]any{"to": nil}, map[string]string{"config.to": "required"}},
		"empty recipients":          {map[string]any{"to": []string{}}, map[string]string{"config.to": "required"}},
		"bad recipient":             {map[string]any{"to": []string{"a@example.com", "nope"}}, map[string]string{"config.to": "invalid"}},
		"recipient with break":      {map[string]any{"to": []string{"a@example.com\r\nBcc: x@example.net"}}, map[string]string{"config.to": "invalid"}},
		"too many recipients":       {map[string]any{"to": smtpManyRecipients(21)}, map[string]string{"config.to": "too_long"}},
		"several problems":          {map[string]any{"host": nil, "port": 0, "to": nil}, map[string]string{"config.host": "required", "config.port": "out_of_range", "config.to": "required"}},
	} {
		t.Run(name, func(t *testing.T) {
			if got := webhookRefused(t, "email", smtpConfig(t, tc.changes)); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("fields = %v, want %v", got, tc.want)
			}
		})
	}
	for name, config := range map[string]string{
		"unknown field": `{"host":"h.example","port":25,"security":"none","from":"a@example.org","to":["b@example.org"],"tls_skip_verify":true}`,
		"not an object": `"smtp"`,
	} {
		if got := webhookRefused(t, "email", config); !reflect.DeepEqual(got, map[string]string{"config": "invalid"}) {
			t.Errorf("%s: fields = %v", name, got)
		}
	}
}

func smtpManyRecipients(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("user%d@example.com", i)
	}
	return out
}

func TestEmailBuildAccepts(t *testing.T) {
	for name, changes := range map[string]map[string]any{
		"gmail":               {"host": "smtp.gmail.com", "port": 465, "security": "tls"},
		"plain from":          {"from": "reef@example.org"},
		"local relay":         {"host": "relay.lan", "security": "none", "port": 25, "username": nil, "password": nil},
		"loopback with login": {"host": "127.0.0.1", "security": "none", "port": 1025},
		"localhost":           {"host": "LocalHost", "security": "none"},
		"ipv6 loopback":       {"host": "::1", "security": "none"},
		"ipv4 host":           {"host": "192.168.1.5"},
		"twenty recipients":   {"to": smtpManyRecipients(20)},
		"no credentials":      {"username": nil, "password": nil},
	} {
		t.Run(name, func(t *testing.T) { webhookBuilt(t, "email", smtpConfig(t, changes), Env{}) })
	}
}

func TestEmailLoopbackMeansTheMachine(t *testing.T) {
	for host, want := range map[string]bool{
		"localhost": true, "LOCALHOST": true, "127.0.0.1": true, "127.0.0.2": true, "::1": true,
		"example.com": false, "192.168.1.5": false, "10.0.0.1": false, "localhost.example.com": false, "0.0.0.0": false, "": false,
	} {
		if got := emailIsLoopback(host); got != want {
			t.Errorf("emailIsLoopback(%q) = %v, want %v", host, got, want)
		}
	}
}
