// Package envkek implements envelope-encrypted secret storage.
//
// The MVP KeyProvider reads a single AES-256-GCM KEK from CLG_KEK_BASE64.
// Store encrypts each payload with a fresh per-secret DEK, wraps that DEK with
// the KEK, and persists ciphertexts in Postgres.
package envkek
