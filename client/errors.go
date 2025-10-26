package client

import "fmt"

var (
	ErrEnvRequired        = fmt.Errorf("environment is required")
	ErrInvalidEnv         = fmt.Errorf("invalid environment only %v are supported", validEnvs)
	ErrInvalidAuthType    = fmt.Errorf("invalid authType only %v are supported", validAuthTypes)
	ErrNilHTTPClient      = fmt.Errorf("http client cannot be nil")
	ErrTLDRequired        = fmt.Errorf("TLD is required")
	ErrAuthTypeRequired   = fmt.Errorf("authType is required")
	ErrCertRequired       = fmt.Errorf("certificate PEM is required when AuthType is TLSA")
	ErrKeyRequired        = fmt.Errorf("key PEM is required when AuthType is TLSA")
	ErrUsernameRequired   = fmt.Errorf("username is required when AuthType is basic")
	ErrPasswordRequired   = fmt.Errorf("password is required when AuthType is basic")
	ErrUnsupportedVersion = fmt.Errorf("unsupported version only %v are supported", validVersions)
	ErrUnsupportedEntity  = fmt.Errorf("unsupported entity only %v are supported", validEntities)
	ErrUnsupportedService = fmt.Errorf("unsupported service only %v are supported", validServices)
)
