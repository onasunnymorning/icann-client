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

	cfg, err := ini.Load([]byte(processed))
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
		val := strings.TrimSpace(key.Value())
		kv[name] = val
	}
	return kv, nil
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
