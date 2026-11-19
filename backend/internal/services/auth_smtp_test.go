package services

import (
	"bufio"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type smtpTestServer struct {
	config    SMTPConfig
	tlsConfig *tls.Config
	done      chan struct{}
	commands  []string
	body      strings.Builder
	plaintext bool
}

func newSMTPTestServer(t *testing.T, mode string, advertiseAuth bool) *smtpTestServer {
	t.Helper()
	certificateServer := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	certificate := certificateServer.TLS.Certificates[0]
	pool := x509.NewCertPool()
	pool.AddCert(certificateServer.Certificate())
	certificateServer.Close()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	host, port, _ := net.SplitHostPort(listener.Addr().String())
	s := &smtpTestServer{
		config:    SMTPConfig{Host: host, Port: port, Username: "canary-user", Password: "canary-password", UseTLS: mode == "implicit"},
		tlsConfig: &tls.Config{RootCAs: pool, ServerName: host, MinVersion: tls.VersionTLS12},
		done:      make(chan struct{}),
	}
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		defer close(s.done)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
		secured := false
		upgrade := func() bool {
			tlsConn := tls.Server(conn, &tls.Config{Certificates: []tls.Certificate{certificate}, MinVersion: tls.VersionTLS12})
			if err := tlsConn.Handshake(); err != nil {
				return false
			}
			conn, secured = tlsConn, true
			return true
		}
		if mode == "implicit" && !upgrade() {
			return
		}
		reader := bufio.NewReader(conn)
		reply := func(line string) { _, _ = fmt.Fprintf(conn, "%s\r\n", line) }
		reply("220 smtp.test ESMTP")
		inData := false
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			command := strings.TrimSpace(line)
			if inData {
				if command == "." {
					inData = false
					reply("250 stored")
				} else {
					s.body.WriteString(line)
				}
				continue
			}
			verb, _, _ := strings.Cut(command, " ")
			s.commands = append(s.commands, verb)
			switch verb {
			case "EHLO":
				reply("250-smtp.test")
				if !secured && (mode == "starttls" || mode == "reject-starttls") {
					reply("250-STARTTLS")
				}
				if advertiseAuth {
					reply("250-AUTH PLAIN")
				}
				reply("250 OK")
			case "STARTTLS":
				if mode == "reject-starttls" {
					reply("454 TLS unavailable")
					continue
				}
				reply("220 start TLS")
				if !upgrade() {
					return
				}
				reader = bufio.NewReader(conn)
			case "AUTH", "MAIL", "RCPT", "DATA":
				if !secured {
					s.plaintext = true
				}
				switch verb {
				case "AUTH":
					reply("235 authenticated")
				case "DATA":
					inData = true
					reply("354 send data")
				default:
					reply("250 OK")
				}
			case "QUIT":
				reply("221 bye")
				return
			default:
				reply("500 unsupported")
			}
		}
	}()
	return s
}

func (s *smtpTestServer) wait(t *testing.T) {
	t.Helper()
	select {
	case <-s.done:
	case <-time.After(3 * time.Second):
		t.Fatal("SMTP connection did not close")
	}
}

func TestSMTPRejectsDowngradeBeforeCredentialsOrResetCode(t *testing.T) {
	for _, tc := range []struct {
		name, mode                string
		auth, username, untrusted bool
	}{
		{name: "no extensions", mode: "plain", username: true},
		{name: "AUTH without STARTTLS including localhost", mode: "plain", auth: true, username: true},
		{name: "unauthenticated relay still requires TLS", mode: "plain"},
		{name: "STARTTLS rejected", mode: "reject-starttls", auth: true, username: true},
		{name: "certificate rejected", mode: "starttls", auth: true, username: true, untrusted: true},
		{name: "AUTH missing after TLS", mode: "starttls", username: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newSMTPTestServer(t, tc.mode, tc.auth)
			if !tc.username {
				s.config.Username = ""
			}
			if tc.untrusted {
				s.tlsConfig.RootCAs = x509.NewCertPool()
			}
			err := sendSMTPMailWithTLSConfig(s.config, "from@example.test", []string{"to@example.test"}, []byte("Subject: test\r\n\r\nRESET_CODE_CANARY\r\n"), s.tlsConfig)
			s.wait(t)
			if err == nil {
				t.Fatal("insecure or unauthenticated SMTP was accepted")
			}
			for _, command := range s.commands {
				if command == "AUTH" || command == "MAIL" || command == "RCPT" || command == "DATA" {
					t.Fatalf("sent %s before the required security checks completed", command)
				}
			}
			if s.body.Len() != 0 || s.plaintext {
				t.Fatal("sensitive SMTP content escaped the security checks")
			}
		})
	}
}

func TestSMTPSendsResetCodeOnlyOverVerifiedTLS(t *testing.T) {
	for _, mode := range []string{"starttls", "implicit"} {
		t.Run(mode, func(t *testing.T) {
			s := newSMTPTestServer(t, mode, true)
			err := sendSMTPMailWithTLSConfig(s.config, "from@example.test", []string{"to@example.test"}, []byte("Subject: test\r\n\r\nRESET_CODE_CANARY\r\n"), s.tlsConfig)
			s.wait(t)
			if err != nil || s.plaintext || !strings.Contains(s.body.String(), "RESET_CODE_CANARY") {
				t.Fatalf("secure SMTP delivery failed: error=%v plaintext=%t", err, s.plaintext)
			}
			if !strings.Contains(strings.Join(s.commands, ","), "AUTH") {
				t.Fatal("configured authentication was skipped")
			}
		})
	}
}
