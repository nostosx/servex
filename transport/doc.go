// Package transport defines protocol-independent contracts. Server ties a
// protocol server into the app lifecycle together with its listen URL;
// Invoker performs a protocol-specific call against a selected address.
// Target resolution and the shared client engine live in servex-contrib.
package transport
