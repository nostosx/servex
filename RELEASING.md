# Releasing servex

Core releases use `vMAJOR.MINOR.PATCH` versions. Patch releases keep the public
API; starting with v1.2.0, minor releases may change it, and every such change
must appear under "Breaking changes" in the changelog with its migration.
This document is a manual release procedure;
the CI workflow validates changes and does not
publish releases or create tags.

## Preparation

1. Choose a module version independently of the Go 1.27.3 toolchain version.
   Core tags use `vMAJOR.MINOR.PATCH`, with no directory prefix. Follow Go's
   major-version import-path rules before planning a v2 release.
2. Review the public API, security boundaries, dependency table in
   [CONTRIBUTING.md](CONTRIBUTING.md#dependency-boundaries) and relevant
   regressions. Check both READMEs against the actual API. Move the selected entries from `Unreleased` into the matching version
   section in [CHANGELOG.md](CHANGELOG.md) and record the release date.
3. Verify the module graph without local replacements. The current core has
   no external requirements after Loader and Client extraction; any future
   kitx adoption must follow the documented package allowlist. Confirm the
   license is included and release contents contain no credentials or local artifacts.
4. With Go 1.27.3 and the documented tool versions, run `task check`,
   `task lint` and `task vulncheck`. Require successful CI for the exact
   release revision. Record service or platform coverage separately from
   compilation-only checks.

## Publication and downstream verification

Publishing requires a separate maintainer decision. Release and independently
verify any new kitx version before adopting it; v1.0.0 is already published.
Publish the core before servex-contrib modules and servex-examples. Create the
chosen core tag only after the release gates pass and publish notes from its
changelog section. Record the exact commit and its CI run; local checks alone
must not be described as hosted CI verification.

From a fresh consumer module outside the development workspace, fetch the
published version with `GOWORK=off`, import a public package and build it
without sibling directories or replacement directives. Verify the published
module version and license. A local check against a sibling checkout does not
prove proxy availability or downstream installation.

After core installation succeeds, servex-contrib can adopt this version, replace
any development snapshots or local replacements, and run its own release gates.
Finally update servex-examples to released dependencies and
verify its documented setup from an independent checkout. Do not advertise
those projects as installable until their own checks pass.

If a published version is faulty, publish a corrected version and document
its impact; do not move an existing tag. Use a Go module retraction when
appropriate and explain the replacement version.
