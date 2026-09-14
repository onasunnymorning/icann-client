# icann-client

[![CI](https://github.com/onasunnymorning/icann-client/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/onasunnymorning/icann-client/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/onasunnymorning/icann-client.svg)](https://pkg.go.dev/github.com/onasunnymorning/icann-client)

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

### Homebrew (CLI)

Once the tap is set up (see below), install the CLI with Homebrew:

```
brew tap onasunnymorning/tap
brew install icann
```

To update:

```
brew update && brew upgrade icann
```

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

The `rri` subpackage follows the same pattern as `mosapi`; both share the same base client and auth configuration so you can reuse credentials easily.

### RRI (library)

Use the RRI client to check registry escrow (Ry Escrow) report status for a date:

```go
import (
	base "github.com/onasunnymorning/icann-client/client"
	"github.com/onasunnymorning/icann-client/rri"
)

cfg := base.Config{ TLD: "example", AuthType: base.AUTH_TYPE_BASIC, Username: "user", Password: "pass" }
rc, _ := rri.New(cfg)
st, err := rc.GetRyEscrowReportStatus(context.Background(), time.Date(2025,10,22,0,0,0,0,time.UTC))
if err != nil { /* handle */ }
fmt.Println(st.Status) // "received" or "pending"
```

Submit an RDE (registry escrow) report. The report id is taken from the document
itself, and the bytes are transmitted verbatim so any hash or signature over the
report stays valid:

```go
report, _ := os.ReadFile("example-20250101-full.xml")

meta, err := rri.ParseRyEscrowReport(report)
if err != nil { /* not a usable report */ }
if err := meta.Validate(rri.ValidationOptions{TLD: cfg.TLD}); err != nil {
	// Catches, locally, what ICANN would reject: wrong TLD, future dates,
	// duplicate counts.
}

res, err := rc.SubmitRyEscrowReport(context.Background(), meta.ID, report)
```

**HTTP 200 does not mean the report was accepted.** ICANN answers both 200 and
400 with a result envelope, and only result code 1000 is an acceptance. A nil
error from `SubmitRyEscrowReport` means code 1000; a rejection comes back as a
`*rri.ResultError`:

```go
if err != nil {
	if code, ok := rri.ResultCodeOf(err); ok {
		// Rejected by ICANN, e.g. 2006 (id mismatch) or 2007 (interface disabled).
		fmt.Println("rejected with code", code, "retryable:", rri.IsRetryable(err))
	} else {
		// Transport or HTTP failure; a *client.HTTPError carries the status.
	}
}
```

Submitting a report whose id was already accepted overwrites the previous one,
so the call is safe to repeat.

The two Specification 3 monthly reports (Section 3 of the draft) work the same
way, over `SubmitMonthlyReport`. Their filenames carry everything the URL needs:

```go
name := "example-transactions-202501.csv"
report, _ := os.ReadFile(name)

month, typ := rri.ParseMonthlyFilename(name)
if month == "" || typ == "" { /* pass them explicitly instead */ }

meta, _ := rri.ParseMonthlyReport(report)
if err := meta.Validate(rri.MonthlyValidationOptions{TLD: cfg.TLD, Month: month, Type: typ}); err != nil {
	// Catches, locally, what ICANN would reject: a bad encoding, a ragged
	// CSV, negative values, and a totals line that does not add up.
}

res, err := rc.SubmitMonthlyReport(context.Background(), typ, month, report)
```

The same `ResultCodeOf`/`IsRetryable` branch applies, and `rri.ResultHint(code)`
returns a one-line explanation for the codes a backfill actually hits — most
usefully 2002, where the month's cut-off date has passed and no retry will help.
`GetMonthlyReportStatus` reports whether ICANN already holds a given month.

### MOSAPI URL structure

MOSAPI endpoints are versioned and scoped by entity and TLD/registrar ID. This library composes the path automatically from `Config.Entity`, `Config.TLD`, and `Config.Version`.

- Base path format: `/<entity>/<tld-or-registrar-id>/<version>`
- Example (registry entity, TLD "example", v2): `/ry/example/v2/monitoring/state`

### MOSAPI sessions

MOSAPI is session based, unlike RRI. Credentials are accepted only at the
unversioned `/<entity>/<tld>/login` endpoint, which returns a session cookie
scoped to `/<entity>/<tld>` and all sub-paths; the versioned endpoints
authenticate with that cookie, or with a TLS client certificate. A request that
arrives with neither is answered:

```
401 The client could not be authenticated using any of the available methods:
TLS-Client-Authentication or Session Cookie.
```

The client handles this for you. A `mosapi.Client` using basic auth logs in
before its first request, reuses the session for subsequent ones, and renews it
once if the server reports it expired. Certificate authentication skips the
login entirely, since the versioned endpoints accept a certificate directly.

`Login` and `Logout` are exported for callers that want to manage the session
themselves, and `HasSession` reports whether one is held:

```go
msc, _ := mosapi.New(cfg)
if err := msc.Login(ctx); err != nil {
	return err
}
defer msc.Logout(ctx)
```

Two ICANN-side limits are worth knowing. A session expires 15 minutes after it
is created, and **only one concurrent session is permitted per account** — a new
login terminates the account's oldest session. So a long-running process should
hold one client and reuse it, and two tools running against the same account
will evict each other.

### Domain METRICA (library)

Use the MOSAPI client to retrieve METRICA (formerly DAAR) reports:

```go
ctx := context.Background()
msc, _ := mosapi.New(cfg)

// Latest report for the configured TLD/entity/version
latest, err := msc.GetMetricaLatest(ctx)
if err != nil { /* handle */ }
fmt.Println("last modified:", latest.LastModified)

// Report for a specific date (YYYY-MM-DD)
rep, err := msc.GetMetricaByDate(ctx, "2024-02-20")
if err != nil { /* handle */ }

// List available reports (optionally filtered by start/end)
lists, err := msc.ListMetricaReports(ctx, "2025-01-01", "2025-01-31")
if err != nil { /* handle */ }
_ = lists
```

Notes:
- For latest and date-specific calls, the HTTP Last-Modified header is exposed as `LastModified` on the response.

## CLI

This repo includes a Cobra-based CLI at `cmd/icann`.
### Homebrew tap setup (maintainers)

We publish a Homebrew formula via GoReleaser to the tap repository `onasunnymorning/homebrew-tap`.

One-time setup:

1. Create the repo `onasunnymorning/homebrew-tap` (public).
2. In this repo (icann-client), add a repo secret named `TAP_GITHUB_TOKEN` with a Fine‑grained PAT that has Contents: Read & write on BOTH repositories: `onasunnymorning/icann-client` and `onasunnymorning/homebrew-tap` (grant org SSO if applicable). Classic PAT with `repo` scope also works if you prefer.
3. Cut a new tag (e.g., `v0.1.1` or re-run the failed workflow after updating the secret); the release workflow will generate/update the formula automatically.

Users then run `brew tap onasunnymorning/tap && brew install icann`.

Notes:
- The legacy command groups `mosapi` and `rri` are deprecated; use the flattened commands under `icann get ...` instead.

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
; For encrypted private keys, add:
; key_passphrase = your_passphrase_here
```

Flags always override file/env values. For TLSA, prefer certificate_pem and key_pem.

### Commands

- Get TLD monitoring state

```
./icann get tld status --tld example \
	--credentials-file ~/.icann/credentials
```

Available flags on `icann get ...` commands:

- `--tld` TLD (required if not provided in credentials)
- `--env` prod|ote
- `--auth` basic|tlsa
- `--username` / `--password` (for basic)
- `--cert-pem` / `--key-pem` (for tlsa)
- `--key-passphrase` (for encrypted private keys)
- `--api-version` (default v2)
- `--entity` (default ry)
- `--profile` (default env ICANN_PROFILE or 'default')
- `--credentials-file` (default env ICANN_SHARED_CREDENTIALS_FILE or `~/.icann/credentials`)

Global flags:

- `--version` Show version information

Output is pretty-printed JSON of the `StateResponse`.

- Domain METRICA

	- Latest report

	```
	./icann get metrica latest --tld example \
		--credentials-file ~/.icann/credentials
	```

	- Report for a specific date

	```
	./icann get metrica date 2024-02-20 --tld example \
		--credentials-file ~/.icann/credentials
	```

	- List available reports (optional filters)

	```
	./icann get metrica lists --tld example \
		--start-date 2025-01-01 --end-date 2025-01-31 \
		--credentials-file ~/.icann/credentials
	```

	Output is pretty-printed JSON matching the MOSAPI spec. For latest and per-date queries, the HTTP Last-Modified header is captured in the `LastModified` field of the response object.

	- RRI

		- Check Ry Escrow report status for a date

		```
		./icann get escrow status --tld example \
			--date 2025-10-22 \
			--credentials-file ~/.icann/credentials
		```

		Output is a small JSON object like:

		```json
		{
			"Type": "ry-escrow",
			"TLD": "example",
			"Date": "2025-10-22T00:00:00Z",
			"Status": "received"
		}
		```

		- Submit RDE (registry escrow) reports

		```
		./icann submit escrow report ./reports/ --tld example \
			--credentials-file ~/.icann/credentials
		```

		Each argument may be a report file, a directory (its `*.xml` entries are
		submitted in lexical order, which for date-stamped names is chronological),
		or a glob. The report id comes from the `<rdeReport:id>` element of each
		file. Every report is parsed and validated before anything is sent, so a
		bad file fails the run rather than the ninth upload.

		Reports are submitted sequentially over a single connection. ICANN
		rate-limits on authentication, so the batch is deliberately not
		parallelised and `--delay` defaults to `1s`.

		| Flag | Default | Purpose |
		| --- | --- | --- |
		| `--id` | from the file | Report id to submit under; a single file only |
		| `--dry-run` | `false` | Validate and report what would be sent, without submitting |
		| `--delay` | `1s` | Pause between submissions; `0` disables |
		| `--stop-on-error` | `false` | Stop at the first failure instead of continuing |
		| `--no-preflight` | `false` | Skip local parsing; requires `--id` and a single file |
		| `--skip-received` | `false` | Skip reports already received (doubles the request count) |

		Output is a single JSON envelope, the same shape for one report or twelve:

		```json
		{
			"tld": "example",
			"dryRun": false,
			"total": 2,
			"succeeded": 1,
			"failed": 1,
			"skipped": 0,
			"results": [
				{
					"file": "reports/example-20250101-full.xml",
					"status": "accepted",
					"id": "example-20250101-full",
					"kind": "FULL",
					"watermark": "2025-01-01T00:00:00Z",
					"url": "https://ry-api.icann.org/report/registry-escrow-report/example/example-20250101-full",
					"httpStatus": 200,
					"resultCode": 1000,
					"message": "No ERRORs were found, and the report has been accepted by ICANN."
				},
				{
					"file": "reports/example-20250102-diff.xml",
					"status": "rejected",
					"id": "example-20250102-diff",
					"kind": "DIFF",
					"httpStatus": 400,
					"resultCode": 2205,
					"message": "Report regarding a differential deposit received when a full deposit was expected"
				}
			]
		}
		```

		A per-file progress line goes to stderr so stdout stays a single JSON
		document. `status` is one of `accepted`, `rejected`, `error`, `skipped`
		or `validated`; the command exits non-zero if any report failed.

		Re-running is safe: a report whose id was already accepted simply
		overwrites the previous submission.

		- Submit Specification 3 monthly reports (transactions, activity)

		```
		./icann submit monthly ./monthly-reports/ --tld example \
			--credentials-file ~/.icann/credentials
		```

		The two monthly CSV reports required by Specification 3 of the gTLD Base
		Registry Agreement go to separate endpoints
		(`/report/registrar-transactions/<tld>/<yyyy-mm>` and
		`/report/registry-functions-activity/<tld>/<yyyy-mm>`), but one command
		handles both. Each file's type is detected from its CSV header line and
		its month is read from the filename. Both the Specification 3 convention
		(`<tld>-transactions-<yyyymm>.csv`) and the looser shapes providers hand
		over in practice (`registrar-transactions-2026-08.csv`) are understood:
		the type is any `transactions` or `activity` token in the name, and the
		month is the last `YYYYMM` or `YYYY-MM` in it. A
		single run may therefore mix both report types across many months — which
		is what a provider-migration backfill looks like.

		If a filename says one report type and the CSV header says the other, the
		run fails rather than guessing: that is the easiest way to file the wrong
		report against the wrong endpoint.

		Pre-flight runs before anything is sent and re-adds every numeric column
		against the totals line, catching locally what would otherwise come back
		as result code 2101:

		```
		example-transactions-202501.csv: column "net-adds-1-yr": totals line says 15,
		but the 2 data lines sum to 17 (result code 2101)
		```

		It also checks UTF-8 encoding (2105), CSV structure (2001), negative
		values (2003), the `Totals` line's empty second field (2103), the `tld`
		column, and that the month has ended (2004). The CSV itself is sent
		byte for byte as it sits on disk.

		| Flag | Default | Purpose |
		| --- | --- | --- |
		| `--month` | from the filename | Month in `YYYY-MM` form; a single file only |
		| `--type` | from the CSV header | Force `transactions` or `activity` |
		| `--dry-run` | `false` | Validate and report what would be sent, without submitting |
		| `--delay` | `1s` | Pause between submissions; `0` disables |
		| `--stop-on-error` | `false` | Stop at the first failure instead of continuing |
		| `--no-preflight` | `false` | Skip local parsing; requires `--month`, `--type` and a single file |
		| `--skip-received` | `false` | Skip months already received (doubles the request count) |

		Output uses the same envelope as `submit escrow report`, with `type` and
		`month` in place of `id`, and a `hint` on rejections that have an
		actionable cause:

		```json
		{
			"file": "monthly-reports/example-transactions-202501.csv",
			"status": "rejected",
			"type": "transactions",
			"month": "2025-01",
			"httpStatus": 400,
			"resultCode": 2002,
			"message": "A report for that month already exists and the cut-off date has passed",
			"hint": "the cut-off date for this month has passed, so ICANN will not accept a replacement; contact ICANN Global Support to have it reopened"
		}
		```

		Before a month's cut-off date a report may be replaced as many times as
		needed, so re-running a partial backfill is safe. After the cut-off ICANN
		rejects the replacement with result code 2002, which no client can work
		around.

Notes:
- Runtime errors (e.g., HTTP 4xx/5xx) do not print the CLI usage banner.
- Errors include the HTTP method and full URL to aid debugging.

## Versioning

## Stability and Versioning

- The module follows SemVer. While in v0, minor versions (v0.x) may include breaking changes.
- Public API stability will be guaranteed starting at v1.0.0. We’ll avoid breaking changes in v0 unless necessary and document them in the Changelog.
- CLI deprecations: legacy command groups `mosapi`/`rri` remain available but hidden and deprecated; use the flattened `icann get ...` commands.
- The module path is `github.com/onasunnymorning/icann-client` and will remain for v1. Major versions `v2+` will use the Go Modules path suffix convention.

See `CHANGELOG.md` for detailed changes.

## Roadmap

- High-level MOSAPI resource methods (e.g., health, reports, domain operations)
- Further RRI interfaces (DNS/DNSSEC reports)
- Retries and backoff (error types landed: `client.HTTPError`, `rri.ResultError`)
- Context-aware helpers and request builders

