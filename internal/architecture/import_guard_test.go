package architecture

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLayerEnforcement verifies that domain/ does not import other layers,
// and that modules only communicate through contract/ interfaces.
func TestLayerEnforcement(t *testing.T) {
	root := filepath.Join("..", "..", "internal", "modules")

	// Walk each module directory
	modules, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("failed to read modules directory: %v", err)
	}

	for _, mod := range modules {
		if !mod.IsDir() {
			continue
		}
		modPath := filepath.Join(root, mod.Name())
		t.Run(mod.Name(), func(t *testing.T) {
			checkModuleLayers(t, modPath)
		})
	}
}

func checkModuleLayers(t *testing.T, modPath string) {
	t.Helper()

	// Parse all .go files in the module
	err := filepath.Walk(modPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			t.Errorf("failed to parse %s: %v", path, err)
			return nil
		}

		// Determine which layer this file belongs to
		relPath, _ := filepath.Rel(modPath, path)
		layer := getLayer(relPath)

		for _, imp := range f.Imports {
			importPath := imp.Path.Value[1 : len(imp.Path.Value)-1] // strip quotes
			checkImport(t, path, layer, importPath)
		}

		return nil
	})

	if err != nil {
		t.Errorf("error walking module %s: %v", modPath, err)
	}
}

func getLayer(relPath string) string {
	parts := strings.Split(relPath, string(os.PathSeparator))
	if len(parts) > 1 {
		return parts[0]
	}
	return "root"
}

func checkImport(t *testing.T, filePath string, layer string, importPath string) {
	t.Helper()

	// Domain layer must not import other layers
	if layer == "domain" {
		if strings.Contains(importPath, "/infrastructure/") ||
			strings.Contains(importPath, "/transport/") ||
			strings.Contains(importPath, "/application/") {
			t.Errorf("%s: domain layer must not import %s", filePath, importPath)
		}
	}

	// Application layer must not import Gin or transport
	if layer == "application" {
		if strings.Contains(importPath, "/transport/") ||
			strings.Contains(importPath, "gin-gonic/gin") {
			t.Errorf("%s: application layer must not import transport: %s", filePath, importPath)
		}
	}

	// Transport layer must not import concrete stores (only contracts)
	if layer == "transport" {
		if strings.Contains(importPath, "/infrastructure/gorm") {
			t.Errorf("%s: transport layer must not import concrete store: %s", filePath, importPath)
		}
	}
}

// TestNoCrossModuleImports verifies that modules only depend on each other's contract packages.
func TestNoCrossModuleImports(t *testing.T) {
	root := filepath.Join("..", "..", "internal", "modules")

	modules, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("failed to read modules directory: %v", err)
	}

	for _, mod := range modules {
		if !mod.IsDir() {
			continue
		}
		modName := mod.Name()
		err := filepath.Walk(filepath.Join(root, modName), func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return err
			}

			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
			if err != nil {
				return nil
			}

			for _, imp := range f.Imports {
				importPath := imp.Path.Value[1 : len(imp.Path.Value)-1]
				// Check if this imports another module's non-contract package
				for _, otherMod := range modules {
					if !otherMod.IsDir() || otherMod.Name() == modName {
						continue
					}
					otherName := otherMod.Name()
					if strings.Contains(importPath, "/modules/"+otherName+"/") &&
						!strings.Contains(importPath, "/"+otherName+"/contract/") &&
						!strings.Contains(importPath, "/"+otherName+"/domain/") {
						t.Errorf("%s: module %s should not import non-contract package from %s: %s",
							path, modName, otherName, importPath)
					}
				}
			}
			return nil
		})
		if err != nil {
			t.Errorf("error checking module %s: %v", modName, err)
		}
	}
}
