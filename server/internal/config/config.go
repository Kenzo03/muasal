// Package config reads the app's settings from environment variables.
package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/kenzo03/muasal/server/internal/mail"
)

// Config holds every setting the binary reads at start.
type Config struct {
	DatabaseURL         string      // app role (least privilege)
	MigrateDatabaseURL  string      // owner role; only migrations use it
	PublicURL           string      // the origin users open, e.g. https://muasal.example.com
	ListenAddr          string      // default ":8080"
	AttachmentsDir      string      // ATTACHMENTS_DIR, default /data/attachments
	BackupsDir          string      // BACKUPS_DIR, default /backups: the backup service's volume (FSD §19.4)
	AttachmentMaxBytes  int64       // 25 MB per file (FSD §8.7); the admin setting comes later
	SecretKey           []byte      // APP_SECRET_KEY: 32 bytes, base64; seals AI API keys (R-AI-3). Optional without BYOK.
	AskLogRetentionDays int         // ASK_LOG_RETENTION_DAYS: 365 by default; 0 keeps the Ask log for good (FSD §15.4)
	SMTP                mail.Config // SMTP_*: email notifications (MSL-10); off without SMTP_HOST
}

// Load reads settings through getenv (os.Getenv in production). Invalid
// values fail at start rather than at the first request.
func Load(getenv func(string) string) (Config, error) {
	c := Config{
		DatabaseURL:        getenv("DATABASE_URL"),
		MigrateDatabaseURL: getenv("MIGRATE_DATABASE_URL"),
		PublicURL:          strings.TrimRight(getenv("PUBLIC_URL"), "/"),
		ListenAddr:         getenv("LISTEN_ADDR"),
		AttachmentsDir:     getenv("ATTACHMENTS_DIR"),
		AttachmentMaxBytes: 25 << 20,
	}
	if c.ListenAddr == "" {
		c.ListenAddr = ":8080"
	}
	if c.BackupsDir = getenv("BACKUPS_DIR"); c.BackupsDir == "" {
		c.BackupsDir = "/backups"
	}
	if c.AttachmentsDir == "" {
		c.AttachmentsDir = "/data/attachments"
	}
	var errs []error
	if c.DatabaseURL == "" {
		errs = append(errs, errors.New("DATABASE_URL is required"))
	}
	if k := getenv("APP_SECRET_KEY"); k != "" {
		key, err := base64.StdEncoding.DecodeString(k)
		if err != nil || len(key) != 32 {
			errs = append(errs, errors.New("APP_SECRET_KEY must be 32 random bytes in base64, e.g. from `openssl rand -base64 32`"))
		}
		c.SecretKey = key
	}
	c.AskLogRetentionDays = 365
	if v := getenv("ASK_LOG_RETENTION_DAYS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			errs = append(errs, fmt.Errorf("ASK_LOG_RETENTION_DAYS must be a whole number of days, 0 to keep for good, got %q", v))
		}
		c.AskLogRetentionDays = n
	}
	c.SMTP = mail.Config{Host: getenv("SMTP_HOST"), Username: getenv("SMTP_USERNAME"), Password: getenv("SMTP_PASSWORD"),
		From: getenv("SMTP_FROM"), TLS: strings.ToLower(getenv("SMTP_TLS"))}
	if c.SMTP.TLS == "" {
		c.SMTP.TLS = "starttls"
	}
	if c.SMTP.Port = 587; c.SMTP.TLS == "tls" {
		c.SMTP.Port = 465
	}
	if v := getenv("SMTP_PORT"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 65535 {
			errs = append(errs, fmt.Errorf("SMTP_PORT must be a port number, got %q", v))
		}
		c.SMTP.Port = n
	}
	if c.SMTP.On() && (c.SMTP.From == "" || !strings.Contains(c.SMTP.From, "@")) {
		errs = append(errs, errors.New("SMTP_FROM must be the sender address, e.g. muasal@example.com, when SMTP_HOST is set"))
	}
	if c.SMTP.TLS != "starttls" && c.SMTP.TLS != "tls" && c.SMTP.TLS != "none" {
		errs = append(errs, fmt.Errorf("SMTP_TLS is starttls, tls or none, got %q", c.SMTP.TLS))
	}
	if u, err := url.Parse(c.PublicURL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.Path != "" {
		errs = append(errs, fmt.Errorf("PUBLIC_URL must be an origin such as https://muasal.example.com, got %q", c.PublicURL))
	}
	return c, errors.Join(errs...)
}

// SecureCookies reports whether session cookies need the Secure flag.
func (c Config) SecureCookies() bool { return strings.HasPrefix(c.PublicURL, "https://") }
