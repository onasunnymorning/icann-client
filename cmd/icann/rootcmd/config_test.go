package rootcmd

import (
	"strings"
	"testing"
)

func TestDescribeSecretNeverRevealsTheValue(t *testing.T) {
	const secret = "s3cr#t-with-hash"
	got := describeSecret(secret)
	if strings.Contains(got, secret) {
		t.Fatalf("describeSecret() = %q, which leaks the secret", got)
	}
	if !strings.Contains(got, "length 16") {
		t.Errorf("describeSecret() = %q, want it to report the length", got)
	}
	// Pinned so the printed digest can be compared against
	//   printf '%s' 's3cr#t-with-hash' | shasum -a 256 | cut -c1-8
	if !strings.Contains(got, "sha256:2e9f62ef") {
		t.Errorf("describeSecret() = %q, want sha256:2e9f62ef", got)
	}
	if describeSecret("") != "(not set)" {
		t.Errorf("describeSecret(\"\") = %q", describeSecret(""))
	}
}

func TestDescribeSecretDistinguishesTruncation(t *testing.T) {
	// The symptom this command exists to catch: a password silently cut short
	// must not share a digest or length with the intended value.
	full := describeSecret("s3cr#t-with-hash")
	cut := describeSecret("s3cr")
	if full == cut {
		t.Error("a truncated password renders identically to the full one")
	}
}

func TestSuspiciousSecret(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{name: "clean", in: "abc123", want: ""},
		{name: "hash", in: "abc#123", want: "verbatim"},
		{name: "semicolon", in: "abc;123", want: "verbatim"},
		{name: "stray quote", in: `"abc`, want: "quote"},
		{name: "trailing space", in: "abc ", want: "whitespace"},
		{name: "empty", in: "", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := suspiciousSecret(tt.in)
			if tt.want == "" {
				if got != "" {
					t.Errorf("suspiciousSecret(%q) = %q, want no note", tt.in, got)
				}
				return
			}
			if !strings.Contains(got, tt.want) {
				t.Errorf("suspiciousSecret(%q) = %q, want a note mentioning %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestDescribePEMDoesNotPrintKeyMaterial(t *testing.T) {
	pem := `-----BEGIN RSA PRIVATE KEY-----\nMIIEowIBAAKCAQEASECRET\n-----END RSA PRIVATE KEY-----`
	got := describePEM(pem)
	if strings.Contains(got, "SECRET") {
		t.Fatalf("describePEM() = %q, which leaks key material", got)
	}
	if !strings.Contains(got, "BEGIN RSA PRIVATE KEY") {
		t.Errorf("describePEM() = %q, want the header line", got)
	}
}
