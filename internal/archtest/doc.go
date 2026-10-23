// Package archtest enforces the dependency rules of the core module. Its
// tests run "go list -deps -test -json ./..." at the module root and check
// that the core depends only on the standard library and kitx. Non-test imports
// must also follow the servex and kitx tables in CONTRIBUTING.md.
package archtest
