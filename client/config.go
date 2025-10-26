package client

import "slices"

type Config struct {
	// TLD is the top-level domain for which the MOSAPI client is being configured.
	TLD string
	// AuthType is the type of authentication to use with the MOSAPI API. This should be one of validAuthTypes.
	AuthType string
	// CertificatePEM is the PEM-encoded certificate used for TLS client authentication when AuthType is AUTH_TYPE_TLSA.
	// It is required in that case.
	CertificatePEM string
	// KeyPEM is the PEM-encoded private key used for TLS client authentication when AuthType is AUTH_TYPE_TLSA.
	// It is required in that case.
	KeyPEM string
	// Username is the username to use for authentication when AuthType is AUTH_TYPE_BASIC and is required in that case.
	Username string
	// Password is the password to use for authentication when AuthType is AUTH_TYPE_BASIC and is required in that case.
	Password string
	// Version is the version of the MOSAPI API to use. It defaults to "v2".
	Version string
	// Entity is the entity for which the MOSAPI client is being configured. This should be one of validEntities.
	Entity string
	// Environment is the environment for which the MOSAPI client is being configured. This should be one of validEnvs.
	Environment string
}

func (c *Config) Validate() error {
	if !slices.Contains(validEnvs, c.Environment) {
		return ErrInvalidEnv
	}
	if !slices.Contains(validVersions, c.Version) {
		return ErrUnsupportedVersion
	}
	if c.TLD == "" {
		return ErrTLDRequired
	}
	if !slices.Contains(validAuthTypes, c.AuthType) {
		return ErrInvalidAuthType
	}
	if !slices.Contains(validEntities, c.Entity) {
		return ErrUnsupportedEntity
	}

	if c.AuthType == AUTH_TYPE_TLSA {
		if c.CertificatePEM == "" {
			return ErrCertRequired
		}
		if c.KeyPEM == "" {
			return ErrKeyRequired
		}
	}
	if c.AuthType == AUTH_TYPE_BASIC {
		if c.Username == "" {
			return ErrUsernameRequired
		}
		if c.Password == "" {
			return ErrPasswordRequired
		}
	}

	return nil
}
