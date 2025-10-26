// Package client provides a shared HTTP client and authentication wiring for
// ICANN APIs (MOSAPI and RRI). It centralizes:
//   - Environment-aware base URLs (prod/ote)
//   - Auth transports: BASIC (username/password) and TLS client cert (TLSA)
//   - Request helpers for composing service-specific relative paths
//
// Service packages (mosapi, rri) compose this base client to expose higher-level
// methods without duplicating transport/auth logic.
package client
