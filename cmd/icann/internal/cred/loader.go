// Package cred provides credential loading functionality for the ICANN CLI.
// It supports AWS-style credential files with profiles and handles multi-line PEM blocks.
package cred

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	ini "gopkg.in/ini.v1"
)

type Record map[string]string

// Load returns the key/value map for a profile from an INI file.
// File path resolution order:
// 1) explicit path
// 2) env ICANN_SHARED_CREDENTIALS_FILE
// 3) ~/.icann/credentials
// Profile resolution order:
// 1) explicit profile
// 2) env ICANN_PROFILE
// 3) "default"
func Load(profile, file string) (Record, error) {
	if file == "" {
		if v := os.Getenv("ICANN_SHARED_CREDENTIALS_FILE"); v != "" {
			file = v
		} else {
			home, _ := os.UserHomeDir()
			file = filepath.Join(home, ".icann", "credentials")
		}
	}
	if profile == "" {
		if v := os.Getenv("ICANN_PROFILE"); v != "" {
			profile = v
		} else {
			profile = "default"
		}
	}
	// Read and pre-process the credentials to collapse multi-line PEM blocks
	raw, readErr := os.ReadFile(file)
	if readErr != nil {
		return nil, readErr
	}
	processed := preprocessPEM(string(raw))

	// IgnoreInlineComment keeps values literal. Without it the parser treats an
	// unquoted "#" or ";" as the start of a comment and silently truncates the
	// value there, which turns a password like "s3cr#t" into "s3cr" and shows up
	// as an authentication failure against the API. Quoting does not help: the
	// default parser truncates inside quotes too and leaves the opening quote in
	// place. Inline comments are instead stripped below, for the few keys whose
	// values cannot legitimately contain those characters.
	cfg, err := ini.LoadSources(ini.LoadOptions{IgnoreInlineComment: true}, []byte(processed))
	if err != nil {
		return nil, err
	}
	if !cfg.HasSection(profile) {
		return nil, fmt.Errorf("credentials profile %q not found in %s", profile, file)
	}
	sec := cfg.Section(profile)
	kv := Record{}
	for _, key := range sec.Keys() {
		name := strings.ToLower(strings.TrimSpace(key.Name()))
		val := key.Value()
		if !literalKeys[name] {
			val = strings.TrimSpace(stripInlineComment(val))
		}
		kv[name] = val
	}
	return kv, nil
}

// literalKeys are the fields whose values are taken exactly as written, because
// they may legitimately contain "#", ";" or significant whitespace. Everything
// else keeps the documented inline-comment style, e.g. "environment = prod ; prod | ote".
//
// A value in this set that must carry leading or trailing whitespace has to be
// quoted in the credentials file; the parser strips the surrounding quotes.
var literalKeys = map[string]bool{
	"password":        true,
	"key_passphrase":  true,
	"username":        true,
	"certificate_pem": true,
	"key_pem":         true,
	"certificate":     true,
	"key":             true,
}

// stripInlineComment removes a trailing "#" or ";" comment from a value.
func stripInlineComment(v string) string {
	if i := strings.IndexAny(v, "#;"); i >= 0 {
		return v[:i]
	}
	return v
}

// preprocessPEM collapses multi-line PEM values for keys certificate_pem and key_pem
// into single-line values with explicit \n separators so the INI parser can handle them.
func preprocessPEM(s string) string {
	scanner := bufio.NewScanner(strings.NewReader(s))
	var out bytes.Buffer
	inKey := "" // "certificate_pem" or "key_pem"
	var acc []string

	writeKey := func(key string, lines []string) {
		if key == "" {
			return
		}
		// Join with \n to preserve newlines; trailing newline not necessary
		joined := strings.Join(lines, "\\n")
		out.WriteString(key)
		out.WriteString(" = ")
		out.WriteString(joined)
		out.WriteString("\n")
	}

	for scanner.Scan() {
		line := scanner.Text()
		trim := strings.TrimSpace(line)
		lower := strings.ToLower(trim)

		if inKey == "" {
			// Detect start of PEM key
			if strings.HasPrefix(lower, "certificate_pem") && strings.Contains(line, "=") {
				// Everything after '=' is part of value; may contain BEGIN line or be empty
				inKey = "certificate_pem"
				parts := strings.SplitN(line, "=", 2)
				val := ""
				if len(parts) == 2 {
					val = strings.TrimSpace(parts[1])
				}
				if val != "" {
					acc = append(acc, val)
				}
				continue
			}
			if strings.HasPrefix(lower, "key_pem") && strings.Contains(line, "=") {
				inKey = "key_pem"
				parts := strings.SplitN(line, "=", 2)
				val := ""
				if len(parts) == 2 {
					val = strings.TrimSpace(parts[1])
				}
				if val != "" {
					acc = append(acc, val)
				}
				continue
			}
			// Not in PEM block: write line as-is
			out.WriteString(line)
			out.WriteString("\n")
			continue
		}

		// Accumulate PEM lines until we reach an END marker
		acc = append(acc, trim)
		if strings.HasPrefix(trim, "-----END ") && strings.HasSuffix(trim, "-----") {
			writeKey(inKey, acc)
			inKey = ""
			acc = acc[:0]
		}
	}

	// If file ended while in PEM, still write what we have
	if inKey != "" && len(acc) > 0 {
		writeKey(inKey, acc)
	}
	return out.String()
}
