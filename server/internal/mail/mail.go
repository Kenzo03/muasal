// Package mail sends notifications by email (MSL-10): a plain-text digest per
// user, through the SMTP server an admin configures. Without SMTP_HOST it is
// off and nothing is sent.
package mail

import (
	"crypto/tls"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

// Config is the SMTP server. TLS is "starttls" (the default: upgrade, and
// refuse a server that cannot), "tls" (implicit, usually port 465) or "none"
// (a relay on the same host or network).
type Config struct {
	Host, Username, Password, From, TLS string
	Port                                int
}

// On reports whether email is set up.
func (c Config) On() bool { return c.Host != "" }

// Send delivers one plain-text message.
func Send(c Config, to, subject, body string) error {
	addr := net.JoinHostPort(c.Host, strconv.Itoa(c.Port))
	var client *smtp.Client
	var err error
	if c.TLS == "tls" {
		conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 30 * time.Second}, "tcp", addr, &tls.Config{ServerName: c.Host})
		if err != nil {
			return err
		}
		_ = conn.SetDeadline(time.Now().Add(time.Minute)) // a stuck server never holds the worker
		client, err = smtp.NewClient(conn, c.Host)
		if err != nil {
			return err
		}
	} else {
		conn, err := net.DialTimeout("tcp", addr, 30*time.Second)
		if err != nil {
			return err
		}
		_ = conn.SetDeadline(time.Now().Add(time.Minute)) // a stuck server never holds the worker
		if client, err = smtp.NewClient(conn, c.Host); err != nil {
			return err
		}
	}
	defer client.Close()
	if c.TLS != "tls" && c.TLS != "none" {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			return errors.New("the SMTP server offers no STARTTLS; set SMTP_TLS=none only for a trusted relay")
		}
		if err = client.StartTLS(&tls.Config{ServerName: c.Host}); err != nil {
			return err
		}
	}
	if c.Username != "" {
		if err = client.Auth(smtp.PlainAuth("", c.Username, c.Password, c.Host)); err != nil {
			return err
		}
	}
	if err = client.Mail(c.From); err != nil {
		return err
	}
	if err = client.Rcpt(to); err != nil {
		return err
	}
	w, err := client.Data()
	if err != nil {
		return err
	}
	if _, err = w.Write(message(c.From, to, subject, body)); err != nil {
		return err
	}
	if err = w.Close(); err != nil {
		return err
	}
	return client.Quit()
}

// message is the RFC 5322 text: UTF-8 subject, plain body, CRLF lines.
func message(from, to, subject, body string) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\nTo: %s\r\nSubject: %s\r\nDate: %s\r\n", from, to, mime.QEncoding.Encode("utf-8", subject), time.Now().Format(time.RFC1123Z))
	b.WriteString("MIME-Version: 1.0\r\nContent-Type: text/plain; charset=utf-8\r\nContent-Transfer-Encoding: 8bit\r\n\r\n")
	for _, line := range strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n") {
		b.WriteString(line + "\r\n") // the Data writer dot-stuffs
	}
	return []byte(b.String())
}
