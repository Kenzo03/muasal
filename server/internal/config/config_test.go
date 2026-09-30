package config

import "testing"

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestLoad(t *testing.T) {
	c, err := Load(env(map[string]string{"DATABASE_URL": "postgres://x", "PUBLIC_URL": "https://muasal.test/"}))
	if err != nil {
		t.Fatal(err)
	}
	if c.ListenAddr != ":8080" || c.PublicURL != "https://muasal.test" || !c.SecureCookies() {
		t.Fatalf("unexpected config: %+v", c)
	}
}

func TestLoadRejectsMissingAndInvalidValues(t *testing.T) {
	for _, publicURL := range []string{"", "muasal.test", "https://muasal.test/app"} {
		if _, err := Load(env(map[string]string{"PUBLIC_URL": publicURL})); err == nil {
			t.Errorf("PUBLIC_URL %q with no DATABASE_URL: want an error", publicURL)
		}
	}
}

func TestHTTPPublicURLMeansInsecureCookies(t *testing.T) {
	c, err := Load(env(map[string]string{"DATABASE_URL": "postgres://x", "PUBLIC_URL": "http://localhost"}))
	if err != nil || c.SecureCookies() {
		t.Fatalf("want insecure cookies for http, got secure=%v err=%v", c.SecureCookies(), err)
	}
}

func TestAttachmentsDefaultToTheDataVolume(t *testing.T) {
	c, err := Load(env(map[string]string{"DATABASE_URL": "postgres://x", "PUBLIC_URL": "http://localhost"}))
	if err != nil || c.AttachmentsDir != "/data/attachments" || c.AttachmentMaxBytes != 25<<20 {
		t.Fatalf("attachments: %q %d %v", c.AttachmentsDir, c.AttachmentMaxBytes, err)
	}
	c, _ = Load(env(map[string]string{"DATABASE_URL": "postgres://x", "PUBLIC_URL": "http://localhost", "ATTACHMENTS_DIR": "/srv/files"}))
	if c.AttachmentsDir != "/srv/files" {
		t.Fatalf("ATTACHMENTS_DIR: %q", c.AttachmentsDir)
	}
}

// R-AI-3: the key that seals AI API keys is optional, but a malformed one
// fails at start instead of at the first BYOK save.
func TestSecretKeyIsOptionalButMustBe32Bytes(t *testing.T) {
	base := map[string]string{"DATABASE_URL": "postgres://x", "PUBLIC_URL": "http://localhost"}
	if c, err := Load(env(base)); err != nil || c.SecretKey != nil {
		t.Fatalf("without APP_SECRET_KEY: %v %v", c.SecretKey, err)
	}
	base["APP_SECRET_KEY"] = "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=" // 32 bytes
	if c, err := Load(env(base)); err != nil || len(c.SecretKey) != 32 {
		t.Fatalf("a 32-byte key: %d %v", len(c.SecretKey), err)
	}
	for _, bad := range []string{"short", "c2hvcnQ="} {
		base["APP_SECRET_KEY"] = bad
		if _, err := Load(env(base)); err == nil {
			t.Errorf("APP_SECRET_KEY %q: want an error", bad)
		}
	}
}

// §15.4: the Ask log keeps questions 365 days unless ASK_LOG_RETENTION_DAYS
// says otherwise; 0 keeps them for good.
func TestAskLogRetention(t *testing.T) {
	base := map[string]string{"DATABASE_URL": "postgres://x", "PUBLIC_URL": "http://localhost"}
	if c, err := Load(env(base)); err != nil || c.AskLogRetentionDays != 365 {
		t.Fatalf("default: %d %v", c.AskLogRetentionDays, err)
	}
	base["ASK_LOG_RETENTION_DAYS"] = "0"
	if c, err := Load(env(base)); err != nil || c.AskLogRetentionDays != 0 {
		t.Fatalf("keep for good: %d %v", c.AskLogRetentionDays, err)
	}
	for _, bad := range []string{"-1", "a year"} {
		base["ASK_LOG_RETENTION_DAYS"] = bad
		if _, err := Load(env(base)); err == nil {
			t.Errorf("ASK_LOG_RETENTION_DAYS %q: want an error", bad)
		}
	}
}

// MSL-10: email is off without SMTP_HOST; with it, a sender is required and
// the port follows the TLS mode unless set.
func TestSMTP(t *testing.T) {
	base := map[string]string{"DATABASE_URL": "postgres://x", "PUBLIC_URL": "https://muasal.test"}
	with := func(kv ...string) map[string]string {
		m := map[string]string{}
		for k, v := range base {
			m[k] = v
		}
		for i := 0; i < len(kv); i += 2 {
			m[kv[i]] = kv[i+1]
		}
		return m
	}
	if c, err := Load(env(base)); err != nil || c.SMTP.On() {
		t.Fatalf("off by default: %+v %v", c.SMTP, err)
	}
	if c, err := Load(env(with("SMTP_HOST", "smtp.example.com", "SMTP_FROM", "muasal@example.com"))); err != nil || c.SMTP.Port != 587 || c.SMTP.TLS != "starttls" {
		t.Fatalf("defaults: %+v %v", c.SMTP, err)
	}
	if c, err := Load(env(with("SMTP_HOST", "smtp.example.com", "SMTP_FROM", "m@example.com", "SMTP_TLS", "TLS"))); err != nil || c.SMTP.Port != 465 {
		t.Fatalf("implicit TLS: %+v %v", c.SMTP, err)
	}
	for _, bad := range []map[string]string{
		with("SMTP_HOST", "smtp.example.com"),
		with("SMTP_HOST", "smtp.example.com", "SMTP_FROM", "m@example.com", "SMTP_TLS", "ssl"),
		with("SMTP_HOST", "smtp.example.com", "SMTP_FROM", "m@example.com", "SMTP_PORT", "0"),
	} {
		if _, err := Load(env(bad)); err == nil {
			t.Errorf("%v: want an error", bad)
		}
	}
}
