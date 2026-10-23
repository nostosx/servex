// Package transport defines the protocol-independent server and client
// contracts: Server ties a protocol server into the app lifecycle together
// with its listen URL, and Client turns an Invoker into endpoints that pick a
// node per call, either from a fixed target or from service discovery.
package transport
