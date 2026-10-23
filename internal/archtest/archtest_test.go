package archtest

import (
	"bytes"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// modulePath is the import path of the core module.
const modulePath = "github.com/nostosx/servex"

// anyCorePackage is the table entry that allows importing every package of
// the core module.
const anyCorePackage = "*"

// allowed maps every package of the core module, relative to modulePath, to
// the core packages its non-test files may import. It mirrors the table in
// CONTRIBUTING.md; keep the two in sync.
//
// Test files may import any core package (for example a reference
// implementation used as a fixture), but like everything else in the core
// module they may only depend on the standard library, the module itself and kitx.
var allowed = map[string][]string{
	"endpoint":              nil,
	"metadata":              nil,
	"errorx":                nil,
	"health":                nil,
	"encoding":              nil,
	"observability/metrics": nil,
	"observability/tracing": nil,
	"app":                   nil,
	"internal/host":         nil,
	"internal/runhook":      nil,
	"config":                {"encoding"},
	"config/file":           {"config", "encoding"},
	"selector":              {"errorx"},
	"discovery":             {"app", "internal/host"},
	"discovery/memory":      {"discovery"},
	"storage/cache":         {"encoding"},
	"storage/cache/memory":  {"storage/cache"},
	"storage/database":      {"app", "observability/metrics", "observability/tracing"},
	"messaging/broker":      {"app", "endpoint", "metadata", "errorx", "internal/runhook"},
	"messaging/broker/memory": {
		"messaging/broker", "endpoint", "errorx", "metadata",
	},
	"messaging/jobqueue": {"app", "endpoint", "metadata", "errorx", "internal/runhook"},
	"messaging/jobqueue/memory": {
		"messaging/jobqueue", "endpoint", "errorx", "metadata",
	},
	"transport": {"app", "endpoint", "metadata", "errorx", "selector", "discovery"},
	"transport/httpx": {
		"transport", "app", "endpoint", "metadata", "errorx",
		"encoding", "health", "selector", "discovery",
	},
	"internal/archtest": {anyCorePackage},
}

// allowedKitx records the external packages used by each core package.
// It does not grant blanket access to other kitx packages.
var allowedKitx = map[string][]string{
	"storage/cache":             {"github.com/nostosx/kitx/singleflight"},
	"storage/cache/memory":      {"github.com/nostosx/kitx/ttlcache"},
	"messaging/jobqueue/memory": {"github.com/nostosx/kitx/backoff"},
	"transport":                 {"github.com/nostosx/kitx/backoff"},
}

// pkg is the subset of the "go list -json" output used by the tests.
type pkg struct {
	ImportPath string
	ForTest    string
	Standard   bool
	Imports    []string
	Error      *struct{ Err string }
}

// listPackages runs "go list -deps -test -json ./..." at the module root
// with workspaces disabled and returns every listed package.
func listPackages(t *testing.T) []pkg {
	t.Helper()
	gomod, err := exec.CommandContext(t.Context(), "go", "env", "GOMOD").Output()
	if err != nil {
		t.Fatalf("go env GOMOD: %v", err)
	}
	root := filepath.Dir(strings.TrimSpace(string(gomod)))

	cmd := exec.CommandContext(t.Context(), "go", "list", "-e", "-deps", "-test", "-json", "./...")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GOWORK=off")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list: %v\n%s", err, stderr.Bytes())
	}

	var pkgs []pkg
	dec := jsontext.NewDecoder(bytes.NewReader(out))
	for {
		var p pkg
		err := json.UnmarshalDecode(dec, &p)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("decode go list output: %v", err)
		}
		pkgs = append(pkgs, p)
	}
	if len(pkgs) == 0 {
		t.Fatal("go list returned no packages")
	}
	return pkgs
}

// corePackage returns importPath relative to modulePath, reporting
// false for packages outside the core module.
func corePackage(importPath string) (string, bool) {
	if importPath == modulePath {
		return ".", true
	}
	return strings.CutPrefix(importPath, modulePath+"/")
}

// basePath strips the " [x.test]" suffix go list adds to test variants.
func basePath(importPath string) string {
	path, _, _ := strings.Cut(importPath, " ")
	return path
}

// TestDependencyBoundary includes tests and kitx transitive dependencies.
func TestDependencyBoundary(t *testing.T) {
	for _, p := range listPackages(t) {
		if p.Error != nil {
			t.Errorf("%s: %s", p.ImportPath, p.Error.Err)
			continue
		}
		if p.Standard {
			continue
		}
		if _, ok := corePackage(basePath(p.ImportPath)); !ok && !strings.HasPrefix(basePath(p.ImportPath), "github.com/nostosx/kitx/") {
			t.Errorf("dependency %s is outside the standard library, %s and kitx", p.ImportPath, modulePath)
		}
	}
}

// TestImportTable checks the non-test imports of every core package
// against the allowed table, and that the table lists exactly the packages
// of the module.
func TestImportTable(t *testing.T) {
	seen := make(map[string]bool)
	for _, p := range listPackages(t) {
		// Test variants and test mains are checked by TestDependencyBoundary;
		// the table only governs the package as other code imports it.
		if p.Standard || p.ForTest != "" || strings.Contains(p.ImportPath, " ") || strings.HasSuffix(p.ImportPath, ".test") {
			continue
		}
		rel, ok := corePackage(p.ImportPath)
		if !ok {
			continue
		}
		seen[rel] = true
		allow, ok := allowed[rel]
		if !ok {
			t.Errorf("package %s is not in the allowed-import table; add it to allowed in internal/archtest and to CONTRIBUTING.md", rel)
			continue
		}
		if slices.Contains(allow, anyCorePackage) {
			continue
		}
		for _, imp := range p.Imports {
			if strings.HasPrefix(imp, "github.com/nostosx/kitx/") && !slices.Contains(allowedKitx[rel], imp) {
				t.Errorf("%s imports unapproved kitx package %s", rel, imp)
			}
			dep, ok := corePackage(imp)
			if !ok {
				continue
			}
			if !slices.Contains(allow, dep) {
				t.Errorf("%s imports %s, which the allowed-import table does not permit (allowed: %v)", rel, dep, allow)
			}
		}
	}
	for rel := range allowed {
		if !seen[rel] {
			t.Errorf("allowed-import table lists %s, which is not a package of the module; remove it", rel)
		}
	}
}

// TestTableEntriesAreCorePackages checks that every allowed import names a
// package that the table itself knows about.
func TestTableEntriesAreCorePackages(t *testing.T) {
	for rel := range allowedKitx {
		if _, ok := allowed[rel]; !ok {
			t.Errorf("kitx import table lists unknown core package %s", rel)
		}
	}
	for rel, allow := range allowed {
		for _, dep := range allow {
			if dep == anyCorePackage {
				continue
			}
			if dep == rel {
				t.Errorf("%s lists itself as an allowed import", rel)
			}
			if _, ok := allowed[dep]; !ok {
				t.Errorf("%s allows %s, which is not in the table", rel, dep)
			}
		}
	}
}
