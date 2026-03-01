package codebase

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
)

// PackageSnapshot represents the semantic metadata for a single Go package.
type PackageSnapshot struct {
	Types      map[string]TypeDefinition `json:"types"`
	Functions  []FuncDefinition          `json:"functions"`
	Interfaces []InterfaceDefinition     `json:"interfaces"`
	Imports    []string                  `json:"imports"`
}

// TypeDefinition describes a Go struct or named type.
type TypeDefinition struct {
	Name   string            `json:"name"`
	Doc    string            `json:"doc,omitempty"`
	Fields map[string]string `json:"fields,omitempty"` // field name -> type
}

// FuncDefinition describes an exported Go function.
type FuncDefinition struct {
	Name    string   `json:"name"`
	Doc     string   `json:"doc,omitempty"`
	Params  []string `json:"params,omitempty"`
	Returns []string `json:"returns,omitempty"`
}

// InterfaceDefinition describes an exported Go interface.
type InterfaceDefinition struct {
	Name    string            `json:"name"`
	Doc     string            `json:"doc,omitempty"`
	Methods map[string]string `json:"methods,omitempty"` // method name -> signature
}

// TakeSnapshot takes a list of relative package paths and extracts their exported symbols.
func TakeSnapshot(pkgPaths []string) (map[string]PackageSnapshot, error) {
	fset := token.NewFileSet()
	snapshot := make(map[string]PackageSnapshot)

	for _, path := range pkgPaths {
		pkgs, err := parser.ParseDir(fset, path, nil, parser.ParseComments)
		if err != nil {
			return nil, fmt.Errorf("failed to parse directory %s: %w", path, err)
		}

		for pkgName, pkg := range pkgs {
			if strings.HasSuffix(pkgName, "_test") {
				continue
			}

			pkgSnap := PackageSnapshot{
				Types:      make(map[string]TypeDefinition),
				Functions:  make([]FuncDefinition, 0),
				Interfaces: make([]InterfaceDefinition, 0),
				Imports:    make([]string, 0),
			}

			// Capture imports (de-duplicated)
			importMap := make(map[string]bool)

			for _, file := range pkg.Files {
				for _, imp := range file.Imports {
					importMap[strings.Trim(imp.Path.Value, `"`)] = true
				}

				for _, decl := range file.Decls {
					switch d := decl.(type) {
					case *ast.GenDecl:
						processGenDecl(d, &pkgSnap)
					case *ast.FuncDecl:
						processFuncDecl(d, &pkgSnap)
					}
				}
			}

			for imp := range importMap {
				pkgSnap.Imports = append(pkgSnap.Imports, imp)
			}

			snapshot[path] = pkgSnap
		}
	}

	return snapshot, nil
}

func processGenDecl(d *ast.GenDecl, snap *PackageSnapshot) {
	if d.Tok != token.TYPE {
		return
	}

	for _, spec := range d.Specs {
		typeSpec, ok := spec.(*ast.TypeSpec)
		if !ok || !typeSpec.Name.IsExported() {
			continue
		}

		typeName := typeSpec.Name.Name
		doc := strings.TrimSpace(d.Doc.Text() + typeSpec.Doc.Text())

		switch t := typeSpec.Type.(type) {
		case *ast.StructType:
			fields := make(map[string]string)
			for _, field := range t.Fields.List {
				for _, name := range field.Names {
					if name.IsExported() {
						fields[name.Name] = typeToString(field.Type)
					}
				}
			}
			snap.Types[typeName] = TypeDefinition{
				Name:   typeName,
				Doc:    doc,
				Fields: fields,
			}

		case *ast.InterfaceType:
			methods := make(map[string]string)
			for _, method := range t.Methods.List {
				for _, name := range method.Names {
					if name.IsExported() {
						methods[name.Name] = typeToString(method.Type)
					}
				}
			}
			snap.Interfaces = append(snap.Interfaces, InterfaceDefinition{
				Name:    typeName,
				Doc:     doc,
				Methods: methods,
			})
		}
	}
}

func processFuncDecl(d *ast.FuncDecl, snap *PackageSnapshot) {
	if !d.Name.IsExported() || d.Recv != nil {
		return // Skip unexported or methods for now (interfaces handled in types)
	}

	params := make([]string, 0)
	if d.Type.Params != nil {
		for _, p := range d.Type.Params.List {
			params = append(params, typeToString(p.Type))
		}
	}

	returns := make([]string, 0)
	if d.Type.Results != nil {
		for _, r := range d.Type.Results.List {
			returns = append(returns, typeToString(r.Type))
		}
	}

	snap.Functions = append(snap.Functions, FuncDefinition{
		Name:    d.Name.Name,
		Doc:     strings.TrimSpace(d.Doc.Text()),
		Params:  params,
		Returns: returns,
	})
}

func typeToString(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.SelectorExpr:
		return fmt.Sprintf("%s.%s", typeToString(t.X), t.Sel.Name)
	case *ast.StarExpr:
		return "*" + typeToString(t.X)
	case *ast.ArrayType:
		return "[]" + typeToString(t.Elt)
	case *ast.MapType:
		return fmt.Sprintf("map[%s]%s", typeToString(t.Key), typeToString(t.Value))
	case *ast.FuncType:
		return "func(...)"
	case *ast.InterfaceType:
		return "interface{...}"
	default:
		return "unknown"
	}
}
