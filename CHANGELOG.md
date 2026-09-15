# Changelog

All notable changes to this project will be documented in this file.

The format is based on Keep a Changelog, and this project adheres to Semantic Versioning.

## [Unreleased]

### Removed
- **Breaking:** `icann get state`. It was an undocumented leftover from before the CLI was flattened that called the same endpoint as `icann get tld status` and printed the same JSON, while resolving credentials through its own copy of the logic. Use `icann get tld status`; the output is unchanged.
- **Breaking:** the `icann mosapi` and `icann rri` command groups. Both were hidden and deprecated with zero subcommands, so they only ever printed help.
- Group commands now reject an unknown subcommand instead of printing help and exiting 0, so a script still calling a removed command fails rather than silently writing help text into its output.

### Added
- `icann get reporting status` and `rri.Client.GetReportingStatus` — `GET /info/status/registry/<tld>`, ICANN's own view of each reporting obligation (`Full`, `Diff`, `Dea`, `PRTR`, `RFAR`, `Registry`) and whether it is currently satisfied. `--issues-only` narrows the output to the unsatisfactory obligations and exits non-zero while any remain. Note that the answer is a snapshot with no date range in it: it reports whether the TLD is currently square with ICANN, not which periods were missed.
- `icann get conformance` and `rri.Client.GetConformanceVersion` — `GET /info/status/conformance-version`. A 404 is a documented answer rather than a failure: it means the server predates the endpoint and conforms to `draft-lozano-icann-registry-interfaces-25` and `draft-icann-registrar-interfaces-15`, which are returned with `inferred: true`.
- `icann get monthly status` — exposes `rri.Client.GetMonthlyReportStatus`, which existed but was reachable only through `submit monthly --skip-received`. `--type` is required; `--month` defaults to the previous complete month.

### Fixed
- `icann get reporting status` returned ICANN's response as an error rather than decoding it. The endpoint was implemented against the XML document in `draft-lozano-icann-registry-interfaces` Section 6, but ICANN serves JSON of a different shape (`{"tld": {"name": ...}, "paths": [{"path", "status"}], "created": ...}`), so a successful 200 surfaced as the baffling `http error: 200` with the body attached. It is now decoded as JSON, and a 200 whose body does not match the expected shape reports the payload as the cause instead of claiming an HTTP failure.
- `icann get reporting status` failed against production ICANN with `406 GET https://ry-api.icann.org/info/status/registry/<tld>`. The client sent `Accept: text/xml`, which ICANN refuses on that endpoint. `draft-lozano-icann-registry-interfaces` requires no `Accept` header and does not define a 406 at all, so neither read endpoint sends one now; the response is parsed as XML whatever Content-Type it arrives with. The status codes distinguish the two cases: the conformance endpoint answered 404 (no such route, decided before content negotiation) while the reporting status endpoint answered 406, which is what showed the route exists and the header was at fault.
- Every MOSAPI command (`icann get tld status`, `icann get metrica latest|date|lists`) failed with `401 ... TLS-Client-Authentication or Session Cookie` when using basic authentication. MOSAPI is session based: credentials are accepted only at the unversioned `/<entity>/<tld>/login` endpoint, and the versioned endpoints authenticate with the session cookie it returns. The client went straight to the versioned endpoint with an `Authorization` header, which MOSAPI does not accept there, and had no cookie jar to hold a session even if one had been issued. RRI is unaffected — it authenticates per request and has no login step.

### Added
- `mosapi.Client` now manages the MOSAPI session: it logs in before the first request, reuses the session across calls, and renews it once if the server reports it expired. Certificate authentication skips the login, since the versioned endpoints accept a TLS client certificate directly.
- `mosapi.Client.Login`, `mosapi.Client.Logout` and `mosapi.Client.HasSession` for callers that want to manage the session explicitly. ICANN permits only one concurrent session per account and expires it after 15 minutes, so a client should be created once and reused.
- `client.Client` now carries a cookie jar, and `client.Client.BaseURL` returns a copy of the configured base URL.

## [v0.4.0] - 2026-09-14

### Added
- RDE (registry escrow) report submission, per `draft-lozano-icann-registry-interfaces` Section 2.3:
  - `rri.Client.SubmitRyEscrowReport` — `PUT /report/registry-escrow-report/<tld>/<id>`, sending the report body verbatim
  - `rri.ParseRyEscrowReport` and `rri.ReportMeta.Validate` — local pre-flight checks (TLD, id, future dates, duplicate counts) that pre-empt ICANN rejections before a request is spent
  - `rri.ResultError` plus result-code constants and the `ResultCodeOf`, `IsRejected` and `IsRetryable` helpers. ICANN returns a result envelope with both HTTP 200 and 400, and only code 1000 is an acceptance, so a 2xx status alone is never treated as success
- New top-level `icann submit` command group, with `icann submit escrow report <file|dir|glob>...`
  - Accepts files, directories and globs; validates every report before submitting any
  - Submits sequentially over a single connection, since ICANN rate-limits on authentication. `--delay` defaults to `1s`
  - Flags: `--id`, `--dry-run`, `--delay`, `--stop-on-error`, `--no-preflight`, `--skip-received`
  - Prints one JSON envelope for both single and batch runs, per-file progress on stderr, and exits non-zero if any report failed

- Specification 3 monthly report submission, per `draft-lozano-icann-registry-interfaces` Section 3:
  - `rri.Client.SubmitMonthlyReport` — `PUT /report/registrar-transactions/<tld>/<yyyy-mm>` and `PUT /report/registry-functions-activity/<tld>/<yyyy-mm>`, sending the CSV verbatim with `Content-Type: text/csv`
  - `rri.Client.GetMonthlyReportStatus` — the matching `HEAD /info/report/...` status check
  - `rri.ParseMonthlyReport`, `rri.DetectReportType`, `rri.ParseMonthlyFilename` and `rri.MonthlyMeta.Validate` — local pre-flight that re-adds every numeric column against the totals line, and checks encoding, CSV structure, negative values and the month
  - Result-code constants for the monthly interfaces (2002, 2003, 2101, 2102, 2103, 2105, 2111) and `rri.ResultHint`, which explains the codes a backfill hits — notably 2002, where the month's cut-off date has passed
- `icann submit monthly <file|dir|glob>...`
  - Detects each file's report type from its CSV header and its month from the filename, so one run can mix both report types across many months; `--type` and `--month` override
  - Filename parsing accepts both the Specification 3 convention (`<tld>-transactions-<yyyymm>.csv`) and the looser shapes providers hand over (`registrar-transactions-2026-08.csv`): any `transactions`/`activity` token plus the last `YYYYMM` or `YYYY-MM` in the name
  - A filename whose type disagrees with its CSV header is an error, not a guess
  - Same batch behaviour as `submit escrow report`: everything validated before the first request, then sequential over a single connection with `--delay` defaulting to `1s`
  - Shares the JSON envelope, with `type`/`month` in place of `id` and a `hint` field on actionable rejections
- `icann config show` prints the configuration a command would run with — credentials file and profile in use, where each value came from, the resolved TLD/environment/auth type and the endpoint URLs. Secrets are never displayed: a password is reported as its length plus the first eight hex digits of its SHA-256, so it can be checked against `printf '%s' 'the-password' | shasum -a 256 | cut -c1-8` without appearing on screen. It also warns when a value looks mangled on the way in.

### Fixed
- Credentials file values were silently truncated at an unquoted `#` or `;`, so a password such as `s3cr#t` was sent as `s3cr` and the API replied `401 Invalid User and/or Password`. Quoting did not help: the parser truncated inside quotes and kept the opening quote. Usernames, passwords, passphrases and PEM blocks are now read verbatim, while the documented trailing `; comment` style still works for `auth_type`, `tld`, `environment`, `version` and `entity`.

### Changed
- The shared response handling for report submissions (read body, parse the result envelope, classify as acceptance, `*rri.ResultError` or `*client.HTTPError`) moved into one internal helper used by both the escrow and monthly endpoints, so the classification order cannot drift between them. No behaviour change; the existing endpoint tests pass unmodified.

### Notes
- New response types carry lowerCamelCase JSON tags. The older `rri.ReportStatus` remains untagged so that `icann get escrow status` output is unchanged; retagging it is deferred to a future breaking release.

## [v0.3.0] - 2025-01-16

### Added
- `--version` flag to display CLI version information
- Version injection via ldflags in GoReleaser builds

### Changed
- Renamed `--version` flag to `--api-version` for API version specification (breaking change for CLI usage)
- Updated CI to test with Go 1.23.x and 1.24.x (removed 1.22.x to match module requirements)
- Updated release workflow to use Go 1.24.x
- Run staticcheck only on Go 1.24.x (module requires Go 1.24.0)

### Added
- `icann config show` prints the configuration a command would run with — credentials file and profile in use, where each value came from, the resolved TLD/environment/auth type and the endpoint URLs. Secrets are never displayed: a password is reported as its length plus the first eight hex digits of its SHA-256, so it can be checked against `printf '%s' 'the-password' | shasum -a 256 | cut -c1-8` without appearing on screen. It also warns when a value looks mangled on the way in.

### Fixed
- Fixed test exit code preservation when filtering deprecated warnings
- Fixed GetStateResponse comment format (ST1020)
- Added missing package comments (ST1000)
- Fixed Homebrew formula path in GoReleaser config
- Fixed CI coverage file handling

## [v0.2.0] - 2025-01-16

### Added
- Support for encrypted private keys with passphrase:
  - Added `KeyPassphrase` field to `Config` struct
  - Added `--key-passphrase` CLI flag
  - Added `key_passphrase` field to credentials file format
  - Support for PKCS#8 encrypted keys (via `github.com/youmark/pkcs8`)
  - Support for RFC 1423 encrypted keys (legacy format)
- Sample credentials file (`credentials.example`) with examples for both basic and TLSA authentication
- Improved error messages for METRICA endpoints when METRICA is not enabled (404 responses)

### Changed
- Updated dependencies: added `github.com/youmark/pkcs8` and `golang.org/x/crypto` for PKCS#8 encrypted key support

### Added
- `icann config show` prints the configuration a command would run with — credentials file and profile in use, where each value came from, the resolved TLD/environment/auth type and the endpoint URLs. Secrets are never displayed: a password is reported as its length plus the first eight hex digits of its SHA-256, so it can be checked against `printf '%s' 'the-password' | shasum -a 256 | cut -c1-8` without appearing on screen. It also warns when a value looks mangled on the way in.

### Fixed
- Fixed PEM format handling in credentials file (multi-line format now properly processed)
- Fixed encrypted private key decryption to support both PKCS#8 and RFC 1423 formats

## [v0.1.0] - 2025-10-26

### Added
- Base helpers:
  - client.DoJSON for JSON requests/responses with typed error handling.
  - client.HTTPError typed error for non-2xx responses (includes status code, method, URL).
- Shared base client (`client`) with:
  - BASIC auth via custom RoundTripper.
  - TLS client certificate ("TLSA") using PEM strings (no file paths).
  - Request helpers (`NewRequest`, `Do`) and base URL support.
- MOSAPI package (`mosapi`) with:
  - Monitoring state endpoint (`GetStateResponse`).
  - Domain METRICA endpoints: `GetMetricaLatest`, `GetMetricaByDate`, `ListMetricaReports`.
  - `Last-Modified` header captured for latest/date calls.
- MOSAPI convenience:
  - StateResponse.LastUpdatedTime(), Incident.StartTimeTime(), Incident.EndTimeTime() helpers.
- RRI package (`rri`) with `GetRyEscrowReportStatus` (uses GET to avoid HTTP/2 HEAD data warnings).
- Cobra-based CLI (`icann`) with flattened UX:
  - `icann get tld status`
  - `icann get metrica latest|date|lists`
  - `icann get escrow status`
  - AWS-style credentials loader at `~/.icann/credentials` supporting PEM strings and profiles.
- CI workflow:
  - Matrix across macOS/Linux/Windows and Go 1.22/1.23.
  - `go vet`, `staticcheck` (on Linux), and race tests (on Linux).
  - Coverage artifact upload.
- Release workflow:
  - Tag-driven releases via GoReleaser (multi-OS/arch binaries).
- Package docs and examples for pkg.go.dev.

### Changed
- BREAKING: mosapi.Client.GetStateResponse now accepts a context.Context parameter.
- BREAKING: Use int64 for Unix timestamp fields in MOSAPI types (LastUpdateApiDb, Incident StartTime/EndTime).
- Flattened CLI; legacy `mosapi` and `rri` groups are deprecated/hidden.

### Added
- `icann config show` prints the configuration a command would run with — credentials file and profile in use, where each value came from, the resolved TLD/environment/auth type and the endpoint URLs. Secrets are never displayed: a password is reported as its length plus the first eight hex digits of its SHA-256, so it can be checked against `printf '%s' 'the-password' | shasum -a 256 | cut -c1-8` without appearing on screen. It also warns when a value looks mangled on the way in.

### Fixed
- Correct MOSAPI monitoring state path per spec.
- Eliminate HTTP/2 "DATA on HEAD" log noise by switching RRI status probe to GET.

[Unreleased]: https://github.com/onasunnymorning/icann-client/compare/v0.4.0...HEAD
[v0.3.0]: https://github.com/onasunnymorning/icann-client/compare/v0.2.0...v0.3.0
[v0.2.0]: https://github.com/onasunnymorning/icann-client/compare/v0.1.0...v0.2.0
[v0.1.0]: https://github.com/onasunnymorning/icann-client/releases/tag/v0.1.0
