# Changelog

All notable changes to this project will be documented in this file.

The format is based on Keep a Changelog, and this project adheres to Semantic Versioning.

## [Unreleased]

- API surface audit in progress; small breaking changes may occur until v1.0.0.
- Docs: badges, stability notes, and examples polishing.
- Optional: Homebrew tap for CLI distribution.

## [v0.1.0] - 2025-10-26

### Added
- Shared base client (`client`) with:
  - BASIC auth via custom RoundTripper.
  - TLS client certificate ("TLSA") using PEM strings (no file paths).
  - Request helpers (`NewRequest`, `Do`) and base URL support.
- MOSAPI package (`mosapi`) with:
  - Monitoring state endpoint (`GetStateResponse`).
  - Domain METRICA endpoints: `GetMetricaLatest`, `GetMetricaByDate`, `ListMetricaReports`.
  - `Last-Modified` header captured for latest/date calls.
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
- Flattened CLI; legacy `mosapi` and `rri` groups are deprecated/hidden.

### Fixed
- Correct MOSAPI monitoring state path per spec.
- Eliminate HTTP/2 "DATA on HEAD" log noise by switching RRI status probe to GET.

[Unreleased]: https://github.com/onasunnymorning/icann-client/compare/v0.1.0...HEAD
[v0.1.0]: https://github.com/onasunnymorning/icann-client/releases/tag/v0.1.0
