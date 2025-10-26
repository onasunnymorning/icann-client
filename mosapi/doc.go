// Package mosapi provides a high-level client for ICANN MOSAPI endpoints.
//
// It composes the shared HTTP/auth client from the parent module (package client)
// so that both MOSAPI and RRI packages can share credentials and transport
// configuration while exposing service-specific methods.
package mosapi
