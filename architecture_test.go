package paperclip_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPurePackagesDoNotImportShellBoundaries(t *testing.T) {
	for _, dir := range []string{"internal/domain", "internal/review"} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
				continue
			}
			path := filepath.Join(dir, entry.Name())
			file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
			if err != nil {
				t.Fatal(err)
			}
			for _, imp := range file.Imports {
				value := strings.Trim(imp.Path.Value, `"`)
				for _, forbidden := range []string{"os", "os/exec", "syscall", "path/filepath"} {
					if value == forbidden {
						t.Fatalf("%s imports shell boundary package %s", path, forbidden)
					}
				}
			}
		}
	}
}

func TestLedgerDoesNotUseHiddenTempFiles(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("internal", "ledger", "repository.go"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), `".paperclip-`) {
		t.Fatal("ledger temp files must not be hidden")
	}
}
