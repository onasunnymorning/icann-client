# icann-client

Go client library for ICANN MOSAPI (and future RRI) with pluggable authentication.

## Features

- Environments: `prod` and `ote`
- Auth:
  - Basic (username/password)
	- TLS client certificate (aka "TLSA" here) via PEM strings
- Sensible defaults (`prod`, `v2`, `ry` entity)

## Install

Add to your `go.mod`:

```
go get github.com/onasunnymorning/icann-client@v0.1.0
```

Until v1, versions are tagged with `v0.x.y` and don’t require a module path suffix.

## Usage

### MOSAPI with shared auth (recommended structure)

```go
import (
	base  "github.com/onasunnymorning/icann-client/client" // shared auth/transport
	mosapi "github.com/onasunnymorning/icann-client/mosapi" // MOSAPI-specific helpers
)

cfg := base.Config{
	TLD:         "example",
	AuthType:    base.AUTH_TYPE_BASIC,
	Username:    "user",
	Password:    "pass",
	Environment: base.ENV_PROD, // or base.ENV_OTE
	Version:     base.V2,
	Entity:      base.EntityRegistry,
}

msc, err := mosapi.New(cfg)
if err != nil { /* handle */ }
// use msc.Client (embedded base client) or add MOSAPI resource methods on `msc`
```

### TLS client certificate ("TLSA") auth (PEM strings)

```go
cfg := base.Config{
	TLD:         "example",
	AuthType:    base.AUTH_TYPE_TLSA,
	CertificatePEM: "-----BEGIN CERTIFICATE-----\n...\n-----END CERTIFICATE-----\n",
	KeyPEM:         "-----BEGIN RSA PRIVATE KEY-----\n...\n-----END RSA PRIVATE KEY-----\n",
	Environment: base.ENV_OTE,
	Version:     base.V2,
	Entity:      base.EntityRegistry,
}

msc, err := mosapi.New(cfg)
// use msc.Do with requests created via msc.NewRequest
```

Notes:
- Provide PEM-encoded certificate and key strings (no file paths). mTLS is configured on the client.
- Defaults are applied for empty `Environment`/`Version`/`Entity` in the base client.

### RRI

An `rri` subpackage is scaffolded and will follow the same pattern; both `mosapi` and `rri` will share the same base client and auth configuration so you can reuse credentials easily.

### MOSAPI URL structure

MOSAPI endpoints are versioned and scoped by entity and TLD/registrar ID. This library composes the path automatically from `Config.Entity`, `Config.TLD`, and `Config.Version`.

- Base path format: `/<entity>/<tld-or-registrar-id>/<version>`
- Example (registry entity, TLD "example", v2): `/ry/example/v2/monitoring/state`

## CLI

This repo includes a Cobra-based CLI at `cmd/icann`.

### Build

```
go build -o icann ./cmd/icann
```

### Credentials

The CLI reads credentials similar to AWS:

- Default file: `~/.icann/credentials` (override with `ICANN_SHARED_CREDENTIALS_FILE`)
- Default profile: `default` (override with `ICANN_PROFILE` or `--profile`)

INI format example (per-TLD profiles):

```
; You can omit tld if the section name equals the TLD
[example]
auth_type = basic            ; basic | tlsa
username  = myuser           ; for basic
password  = mypass           ; for basic
environment = prod           ; default prod
version     = v2             ; default v2
entity      = ry             ; default ry

; TLSA using PEM strings (use \n for newlines or INI multi-line)
[example-tlsa]
auth_type = tlsa
certificate_pem = -----BEGIN CERTIFICATE-----\n...\n-----END CERTIFICATE-----\n
key_pem = -----BEGIN RSA PRIVATE KEY-----\n...\n-----END RSA PRIVATE KEY-----\n
environment = ote
; You can also paste multi-line PEM blocks directly under certificate_pem/key_pem
; without escaping newlines; the loader will preprocess them.
```

Flags always override file/env values. For TLSA, prefer certificate_pem and key_pem.

### Commands

- Get MOSAPI monitoring state

```
./icann mosapi get state --tld example \
	--credentials-file ~/.icann/credentials
```

Available flags on `icann mosapi get state`:

- `--tld` TLD (required if not provided in credentials)
- `--env` prod|ote
- `--auth` basic|tlsa
- `--username` / `--password` (for basic)
- `--cert-pem` / `--key-pem` (for tlsa)
- `--version` (default v2)
- `--entity` (default ry)
- `--profile` (default env ICANN_PROFILE or 'default')
- `--credentials-file` (default env ICANN_SHARED_CREDENTIALS_FILE or `~/.icann/credentials`)

Output is pretty-printed JSON of the `StateResponse`.

Notes:
- Runtime errors (e.g., HTTP 4xx/5xx) do not print the CLI usage banner.
- Errors include the HTTP method and full URL to aid debugging.

## Versioning

- Semantic version tags will be used (start with `v0.1.0`).
- When/if the API stabilizes to v1, the module path will remain the same; major versions `v2+` would require a path suffix per Go Module rules.

## Roadmap

- High-level MOSAPI resource methods (e.g., health, reports, domain operations)
- RRI client
- Retries, backoff, and error types
- Context-aware helpers and request builders

