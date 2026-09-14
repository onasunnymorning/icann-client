package cred

import (
	"os"
	"path/filepath"
	"testing"
)

// writeCreds writes a credentials file and returns its path.
func writeCreds(t *testing.T, content string) string {
	t.Helper()
	f := filepath.Join(t.TempDir(), "credentials")
	if err := os.WriteFile(f, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return f
}

// TestLoadPreservesSecretsVerbatim guards against the parser truncating a
// secret at a "#" or ";", which presents as an authentication failure against
// the API rather than as a parse error.
func TestLoadPreservesSecretsVerbatim(t *testing.T) {
	secrets := []string{
		"plain123",
		"has#hash",
		"has;semi",
		"has spaces",
		"ends#",
		"#starts",
		"lots#of;both#chars",
		`back\slash`,
		"with=equals",
		"with:colon",
		"pa$$w%rd!",
	}

	for _, want := range secrets {
		t.Run(want, func(t *testing.T) {
			f := writeCreds(t, "[radio]\nauth_type = basic\nusername = "+want+"\npassword = "+want+"\ntld = radio\n")
			rec, err := Load("radio", f)
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if got := rec["password"]; got != want {
				t.Errorf("password = %q, want %q", got, want)
			}
			if got := rec["username"]; got != want {
				t.Errorf("username = %q, want %q", got, want)
			}
		})
	}
}

// TestLoadQuotedSecretKeepsWhitespace documents how to store a secret with
// significant leading or trailing whitespace.
func TestLoadQuotedSecretKeepsWhitespace(t *testing.T) {
	f := writeCreds(t, "[radio]\nauth_type = basic\nusername = u\npassword = \"  padded  \"\ntld = radio\n")
	rec, err := Load("radio", f)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if want := "  padded  "; rec["password"] != want {
		t.Errorf("password = %q, want %q", rec["password"], want)
	}
}

// TestLoadKeepsInlineCommentsOnPlainKeys pins the style documented in
// credentials.example, which annotates the non-secret fields inline.
func TestLoadKeepsInlineCommentsOnPlainKeys(t *testing.T) {
	f := writeCreds(t, `[radio]
auth_type = basic
username  = radio_ry
password  = s3cr#t
tld       = radio
environment = prod    ; prod | ote
version     = v2      ; default v2
entity      = ry      ; ry (registry) | rr (registrar)
`)
	rec, err := Load("radio", f)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	for key, want := range map[string]string{
		"environment": "prod",
		"version":     "v2",
		"entity":      "ry",
		"tld":         "radio",
		"auth_type":   "basic",
		"username":    "radio_ry",
		"password":    "s3cr#t", // untouched, unlike the annotated fields above
	} {
		if got := rec[key]; got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
}

func TestLoadMissingProfile(t *testing.T) {
	f := writeCreds(t, "[radio]\nauth_type = basic\n")
	if _, err := Load("nope", f); err == nil {
		t.Error("Load() = nil error, want a missing-profile error")
	}
}

// TestLoadMultiLinePEM checks that the multi-line PEM form documented in
// credentials.example still collapses correctly, since PEM values are now
// taken literally rather than trimmed.
func TestLoadMultiLinePEM(t *testing.T) {
	f := writeCreds(t, `[radio-tlsa]
auth_type = tlsa
tld = radio
certificate_pem = -----BEGIN CERTIFICATE-----
MIIFXTCCAkWgAwIBAgIJAKZ7D
AAAAAAAAAAAAAAAAAAAAAAAAA
-----END CERTIFICATE-----
key_pem = -----BEGIN RSA PRIVATE KEY-----
MIIEowIBAAKCAQEA
-----END RSA PRIVATE KEY-----
environment = ote   ; prod | ote
`)
	rec, err := Load("radio-tlsa", f)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	wantCert := `-----BEGIN CERTIFICATE-----\nMIIFXTCCAkWgAwIBAgIJAKZ7D\nAAAAAAAAAAAAAAAAAAAAAAAAA\n-----END CERTIFICATE-----`
	if rec["certificate_pem"] != wantCert {
		t.Errorf("certificate_pem = %q, want %q", rec["certificate_pem"], wantCert)
	}
	wantKey := `-----BEGIN RSA PRIVATE KEY-----\nMIIEowIBAAKCAQEA\n-----END RSA PRIVATE KEY-----`
	if rec["key_pem"] != wantKey {
		t.Errorf("key_pem = %q, want %q", rec["key_pem"], wantKey)
	}
	if rec["environment"] != "ote" {
		t.Errorf("environment = %q, want %q", rec["environment"], "ote")
	}
}
