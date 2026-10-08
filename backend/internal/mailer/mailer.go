package mailer

import (
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"
)

// Account describes an SMTP endpoint (customer BYO credentials,
// already decrypted by the caller).
type Account struct {
	Host       string
	Port       int
	Username   string
	Password   string
	FromEmail  string
	FromName   string
	TLSMode    string // starttls | ssl | none
	AuthMethod string // login | plain | cram-md5 | none
}

// Message is a fully rendered email ready for delivery.
type Message struct {
	To           string
	Subject      string
	HTML         string
	Text         string            // optional plain-text alternative
	ExtraHeaders map[string]string // List-Unsubscribe, X-Messages, etc.
}

// Send delivers one message over SMTP with a hard deadline.
func Send(acc Account, m Message) error {
	addr := net.JoinHostPort(acc.Host, fmt.Sprint(acc.Port))

	var conn net.Conn
	var err error
	dialTimeout := 15 * time.Second

	switch acc.TLSMode {
	case "ssl":
		conn, err = tls.DialWithDialer(&net.Dialer{Timeout: dialTimeout}, "tcp", addr, &tls.Config{ServerName: acc.Host})
	default:
		conn, err = net.DialTimeout("tcp", addr, dialTimeout)
	}
	if err != nil {
		return fmt.Errorf("smtp dial %s: %w", addr, err)
	}
	defer conn.Close()

	cl, err := smtp.NewClient(conn, acc.Host)
	if err != nil {
		return fmt.Errorf("smtp client: %w", err)
	}
	defer cl.Quit()

	if acc.TLSMode == "starttls" {
		if err = cl.StartTLS(&tls.Config{ServerName: acc.Host, MinVersion: tls.VersionTLS12}); err != nil {
			return fmt.Errorf("starttls: %w", err)
		}
	}

	if acc.AuthMethod != "none" {
		if err = authenticate(cl, acc); err != nil {
			return fmt.Errorf("auth: %w", err)
		}
	}

	if err = cl.Mail(acc.FromEmail); err != nil {
		return fmt.Errorf("mail from: %w", err)
	}
	if err = cl.Rcpt(m.To); err != nil {
		return fmt.Errorf("rcpt to: %w", err)
	}

	w, err := cl.Data()
	if err != nil {
		return fmt.Errorf("data: %w", err)
	}
	defer w.Close()

	_, err = w.Write(buildRFC822(acc, m))
	if err != nil {
		return fmt.Errorf("write body: %w", err)
	}
	return nil
}

func authenticate(cl *smtp.Client, acc Account) error {
	addr := strings.ToLower(acc.Host)
	var auth smtp.Auth
	switch acc.AuthMethod {
	case "cram-md5":
		auth = smtp.CRAMMD5Auth(acc.Username, acc.Password)
	default: // login | plain — PlainAuth is accepted by all major providers
		// over TLS connections (LOGIN is attempted by servers via SASL anyway).
		auth = smtp.PlainAuth("", acc.Username, acc.Password, addr)
	}
	return cl.Auth(auth)
}

// buildRFC822 assembles the raw message with headers.
func buildRFC822(acc Account, m Message) []byte {
	var b strings.Builder
	from := acc.FromEmail
	if acc.FromName != "" {
		from = fmt.Sprintf("%s <%s>", acc.FromName, acc.FromEmail)
	}
	fmt.Fprintf(&b, "From: %s\r\n", from)
	fmt.Fprintf(&b, "To: %s\r\n", m.To)
	fmt.Fprintf(&b, "Subject: %s\r\n", encodeHeader(m.Subject))
	fmt.Fprintf(&b, "Date: %s\r\n", time.Now().Format(time.RFC1123Z))
	fmt.Fprintf(&b, "Message-ID: <%d@emailsender>\r\n", time.Now().UnixNano())
	fmt.Fprintf(&b, "MIME-Version: 1.0\r\n")
	for k, v := range m.ExtraHeaders {
		fmt.Fprintf(&b, "%s: %s\r\n", k, v)
	}

	boundary := fmt.Sprintf("edge_%d", time.Now().UnixNano())
	if m.Text != "" {
		fmt.Fprintf(&b, "Content-Type: multipart/alternative; boundary=%q\r\n\r\n", boundary)
		fmt.Fprintf(&b, "--%s\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s\r\n", boundary, m.Text)
		fmt.Fprintf(&b, "--%s\r\nContent-Type: text/html; charset=UTF-8\r\n\r\n%s\r\n--%s--\r\n", boundary, m.HTML, boundary)
	} else {
		fmt.Fprintf(&b, "Content-Type: text/html; charset=UTF-8\r\n\r\n%s\r\n", m.HTML)
	}
	return []byte(b.String())
}

// encodeHeader keeps non-ASCII subjects RFC2047-compliant.
func encodeHeader(s string) string {
	for _, r := range s {
		if r > 127 {
			return "=?UTF-8?B?" + b64(s) + "?="
		}
	}
	return s
}

func b64(s string) string {
	return base64.StdEncoding.EncodeToString([]byte(s))
}
