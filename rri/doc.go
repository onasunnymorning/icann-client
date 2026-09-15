// Package rri provides a high-level client for ICANN's Registry Reporting
// Interfaces, as defined by draft-lozano-icann-registry-interfaces.
//
// It composes the shared HTTP/auth client from the parent module (package
// client), so that MOSAPI and RRI share credentials and transport
// configuration. Unlike MOSAPI, RRI authenticates every request on its own and
// has no login step.
package rri
