package client

import (
	"testing"
)

func TestConfig_Validate(t *testing.T) {

	tests := []struct {
		name    string
		config  Config
		wantErr error
	}{
		{
			name: "valid config",
			config: Config{
				TLD:         "example.com",
				AuthType:    AUTH_TYPE_BASIC,
				Username:    "user",
				Password:    "pass",
				Version:     "v2",
				Entity:      "ry",
				Environment: "prod",
			},
			wantErr: nil,
		},
		{
			name: "invalid entity",
			config: Config{
				TLD:         "example.com",
				AuthType:    AUTH_TYPE_BASIC,
				Username:    "user",
				Password:    "pass",
				Version:     "v2",
				Entity:      "entity",
				Environment: "prod",
			},
			wantErr: ErrUnsupportedEntity,
		},
		{
			name: "invalid environment",
			config: Config{
				TLD:         "example.com",
				AuthType:    AUTH_TYPE_BASIC,
				Username:    "user",
				Password:    "pass",
				Version:     "v2",
				Entity:      "ry",
				Environment: "invalid_env",
			},
			wantErr: ErrInvalidEnv,
		},
		{
			name: "unsupported version",
			config: Config{
				TLD:         "example.com",
				AuthType:    AUTH_TYPE_BASIC,
				Username:    "user",
				Password:    "pass",
				Version:     "v3",
				Entity:      "ry",
				Environment: "prod",
			},
			wantErr: ErrUnsupportedVersion,
		},
		{
			name: "missing TLD",
			config: Config{
				AuthType:    AUTH_TYPE_BASIC,
				Username:    "user",
				Password:    "pass",
				Version:     "v2",
				Entity:      "ry",
				Environment: "prod",
			},
			wantErr: ErrTLDRequired,
		},
		{
			name: "invalid auth type",
			config: Config{
				TLD:         "example.com",
				AuthType:    "invalid_auth",
				Version:     "v2",
				Entity:      "ry",
				Environment: "prod",
			},
			wantErr: ErrInvalidAuthType,
		},
		{
			name: "missing certificate for TLSA",
			config: Config{
				TLD:         "example.com",
				AuthType:    AUTH_TYPE_TLSA,
				KeyPEM:      "-----BEGIN PRIVATE KEY-----\n...\n-----END PRIVATE KEY-----\n",
				Version:     "v2",
				Entity:      "ry",
				Environment: "prod",
			},
			wantErr: ErrCertRequired,
		},
		{
			name: "missing key for TLSA",
			config: Config{
				TLD:            "example.com",
				AuthType:       AUTH_TYPE_TLSA,
				CertificatePEM: "-----BEGIN CERTIFICATE-----\n...\n-----END CERTIFICATE-----\n",
				Version:        "v2",
				Entity:         "ry",
				Environment:    "prod",
			},
			wantErr: ErrKeyRequired,
		},
		{
			name: "missing username for BASIC",
			config: Config{
				TLD:         "example.com",
				AuthType:    AUTH_TYPE_BASIC,
				Password:    "pass",
				Version:     "v2",
				Entity:      "ry",
				Environment: "prod",
			},
			wantErr: ErrUsernameRequired,
		},
		{
			name: "missing password for BASIC",
			config: Config{
				TLD:         "example.com",
				AuthType:    AUTH_TYPE_BASIC,
				Username:    "user",
				Version:     "v2",
				Entity:      "ry",
				Environment: "prod",
			},
			wantErr: ErrPasswordRequired,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if err != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
