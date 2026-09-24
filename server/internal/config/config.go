// Package config reads the app's settings from environment variables.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// Config holds every setting the binary reads at start.
type Config struct {
	DatabaseURL        string // app role (least privilege)
	MigrateDatabaseURL string // owner role; only migrations use it
	PublicURL          string // the origin users open, e.g. https://muasal.example.com
	ListenAddr         string // default ":8080"
}

// Load reads settings through getenv (os.Getenv in production). Invalid
// values fail at start rather than at the first request.
func Load(getenv func(string) string) (Config, error) {
	c := Config{
		DatabaseURL:        getenv("DATABASE_URL"),
		MigrateDatabaseURL: getenv("MIGRATE_DATABASE_URL"),
		PublicURL:          strings.TrimRight(getenv("PUBLIC_URL"), "/"),
		ListenAddr:         getenv("LISTEN_ADDR"),
	}
	if c.ListenAddr == "" {
		c.ListenAddr = ":8080"
	}
	var errs []error
	if c.DatabaseURL == "" {
		errs = append(errs, errors.New("DATABASE_URL is required"))
	}
	if u, err := url.Parse(c.PublicURL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.Path != "" {
		errs = append(errs, fmt.Errorf("PUBLIC_URL must be an origin such as https://muasal.example.com, got %q", c.PublicURL))
	}
	return c, errors.Join(errs...)
}

// SecureCookies reports whether session cookies need the Secure flag.
func (c Config) SecureCookies() bool { return strings.HasPrefix(c.PublicURL, "https://") }
