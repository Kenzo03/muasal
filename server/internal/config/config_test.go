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
