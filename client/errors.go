package client

import (
	"fmt"
	"strings"
)

// quoteJoin renders a list of accepted values as a human-readable,
// comma-separated, quoted list (e.g. `"basic", "cert"`), instead of Go's
// default slice formatting (`[basic cert]`).
func quoteJoin(vals []string) string {
	quoted := make([]string, len(vals))
	for i, v := range vals {
		quoted[i] = fmt.Sprintf("%q", v)
	}
	return strings.Join(quoted, ", ")
}

var (
	ErrEnvRequired        = fmt.Errorf("environment is required")
	ErrInvalidEnv         = fmt.Errorf("invalid environment: only %s are supported", quoteJoin(validEnvs))
	ErrInvalidAuthType    = fmt.Errorf("invalid auth type: only %s are supported", quoteJoin(validAuthTypes))
	ErrNilHTTPClient      = fmt.Errorf("http client cannot be nil")
	ErrTLDRequired        = fmt.Errorf("TLD is required")
	ErrAuthTypeRequired   = fmt.Errorf("authType is required")
	ErrCertRequired       = fmt.Errorf("certificate PEM is required when AuthType is cert")
	ErrKeyRequired        = fmt.Errorf("key PEM is required when AuthType is cert")
	ErrUsernameRequired   = fmt.Errorf("username is required when AuthType is basic")
	ErrPasswordRequired   = fmt.Errorf("password is required when AuthType is basic")
	ErrUnsupportedVersion = fmt.Errorf("unsupported version: only %s are supported", quoteJoin(validVersions))
	ErrUnsupportedEntity  = fmt.Errorf("unsupported entity: only %s are supported", quoteJoin(validEntities))
	ErrUnsupportedService = fmt.Errorf("unsupported service: only %s are supported", quoteJoin(validServices))
)
