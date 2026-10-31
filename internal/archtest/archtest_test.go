package archtest

import (
	"bytes"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
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

// TestContractOnlyPackages keeps loading, decoding and logger backends out of
// packages whose ownership is explicitly limited to contract declarations.
func TestContractOnlyPackages(t *testing.T) {
	for _, name := range []string{"config", "logger", "encoding", "selector", "observability/metrics", "storage/cache", "storage/vector", "storage/search", "transport", "transport/sse", "transport/websocket", "transport/rpc"} {
		dir := filepath.Join("..", "..", name)
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
				continue
			}
			path := filepath.Join(dir, entry.Name())
			file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
			if err != nil {
				t.Fatal(err)
			}
			for _, declaration := range file.Decls {
				if function, ok := declaration.(*ast.FuncDecl); ok {
					// Function adapters are part of the contract; client execution is not.
					if name == "transport" && function.Name.Name == "Invoke" && function.Recv != nil && len(function.Recv.List) == 1 {
						if receiver, ok := function.Recv.List[0].Type.(*ast.Ident); ok && receiver.Name == "InvokerFunc" {
							continue
						}
					}
					t.Errorf("%s contains implementation %s; move it to servex-contrib", path, function.Name)
				}
			}
		}
	}
}

// allowed maps every package of the core module, relative to modulePath, to
// the core packages its non-test files may import. It mirrors the table in
// CONTRIBUTING.md; keep the two in sync.
//
// Test files may import any core package as a fixture, but like the rest of the core
// module they may only depend on the standard library, the module itself and kitx.
var allowed = map[string][]string{
	"endpoint":              nil,
	"cqrs":                  nil,
	"metadata":              nil,
	"errorx":                nil,
	"health":                nil,
	"encoding":              nil,
	"observability/metrics": nil,
	"observability/tracing": nil,
	"app":                   {"logger"},
	"logger":                nil,
	"di":                    nil,
	"internal/host":         nil,
	"internal/runhook":      nil,
	"config":                nil,
	"selector":              {"errorx"},
	"discovery":             {"app", "internal/host"},
	"storage/cache":         nil,
	"storage/vector":        nil,
	"storage/search":        nil,
	"messaging/broker":      {"app", "endpoint", "metadata", "errorx", "internal/runhook"},
	"messaging/jobqueue":    {"app", "endpoint", "metadata", "errorx", "internal/runhook"},
	"transport":             {"app"},
	"transport/sse":         nil,
	"transport/websocket":   nil,
	"transport/rpc":         nil,
	"internal/archtest":     {anyCorePackage},
}

// allowedKitx records the external packages used by each core package.
// It does not grant blanket access to other kitx packages.
var allowedKitx = map[string][]string{}

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
			if imp == "log/slog" || imp == "database/sql" || imp == "database/sql/driver" || imp == "encoding/json" || imp == "encoding/json/v2" {
				t.Errorf("%s imports concrete backend %s; depend on a core contract", rel, imp)
			}

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

// TestMovedBackendsStayOutsideCore prevents old import paths from regrowing
// implementation packages after their tests and backends move to contrib.
func TestMovedBackendsStayOutsideCore(t *testing.T) {
	for _, name := range []string{
		"transport/httpx", "storage/database", "storage/cache/memory",
		"discovery/memory", "messaging/broker/memory", "messaging/jobqueue/memory",
		"transport/client.go", "storage/cache/loader.go",
	} {
		path := filepath.Join("..", "..", name)
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("backend path %s must remain outside core (stat: %v)", name, err)
		}
	}
}
