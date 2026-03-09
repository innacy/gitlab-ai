package context

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

// Directories to always skip during indexing.
var skipDirs = map[string]bool{
	".git": true, "vendor": true, "node_modules": true,
	".idea": true, ".vscode": true, "__pycache__": true,
	"bin": true, "dist": true, "build": true, ".gradle": true,
	"target": true, ".next": true, ".nuxt": true,
}

// File extensions considered as code files for the tree listing.
var codeExtensions = map[string]bool{
	".go": true, ".py": true, ".js": true, ".ts": true,
	".java": true, ".rs": true, ".rb": true, ".c": true,
	".cpp": true, ".h": true, ".hpp": true, ".cs": true,
	".yaml": true, ".yml": true, ".json": true, ".toml": true,
	".sql": true, ".sh": true, ".bash": true, ".proto": true,
	".tf": true, ".hcl": true, ".vue": true, ".tsx": true,
	".jsx": true, ".kt": true, ".swift": true, ".xml": true,
	".html": true, ".css": true, ".scss": true, ".less": true,
	".md": true, ".dockerfile": true, ".mod": true, ".sum": true,
}

// IndexDirectory scans a local directory and returns a structured code index as markdown.
func IndexDirectory(rootPath string) (string, error) {
	info, err := os.Stat(rootPath)
	if err != nil {
		return "", fmt.Errorf("cannot access %s: %w", rootPath, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s is not a directory", rootPath)
	}

	var sb strings.Builder
	var fileCount, goFileCount int

	// ── File Tree ────────────────────────────────────────────────────────
	sb.WriteString("### File Structure\n\n```\n")
	err = filepath.Walk(rootPath, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return nil // skip errors
		}
		rel, _ := filepath.Rel(rootPath, path)
		if rel == "." {
			return nil
		}

		base := filepath.Base(path)

		if fi.IsDir() {
			if strings.HasPrefix(base, ".") || skipDirs[base] {
				return filepath.SkipDir
			}
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if codeExtensions[ext] || base == "Makefile" || base == "Dockerfile" || base == "go.mod" || base == "go.sum" {
			depth := strings.Count(rel, string(os.PathSeparator))
			indent := strings.Repeat("  ", depth)
			sb.WriteString(fmt.Sprintf("%s%s\n", indent, rel))
			fileCount++
			if ext == ".go" && !strings.HasSuffix(path, "_test.go") {
				goFileCount++
			}
		}

		return nil
	})
	sb.WriteString("```\n\n")

	if err != nil {
		return "", err
	}

	sb.WriteString(fmt.Sprintf("_Total: %d files indexed", fileCount))
	if goFileCount > 0 {
		sb.WriteString(fmt.Sprintf(", %d Go source files", goFileCount))
	}
	sb.WriteString("_\n\n")

	// ── Go Declarations ──────────────────────────────────────────────────
	if goFileCount > 0 {
		sb.WriteString("### Key Declarations\n\n")
		filepath.Walk(rootPath, func(path string, fi os.FileInfo, err error) error {
			if err != nil || fi.IsDir() {
				if fi != nil && fi.IsDir() {
					base := filepath.Base(path)
					if strings.HasPrefix(base, ".") || skipDirs[base] {
						return filepath.SkipDir
					}
				}
				return nil
			}

			ext := filepath.Ext(path)
			if ext == ".go" && !strings.HasSuffix(path, "_test.go") {
				rel, _ := filepath.Rel(rootPath, path)
				sigs := extractGoSignatures(path)
				if len(sigs) > 0 {
					sb.WriteString(fmt.Sprintf("#### `%s`\n\n", rel))
					for _, sig := range sigs {
						sb.WriteString(fmt.Sprintf("- `%s`\n", sig))
					}
					sb.WriteString("\n")
				}
			}
			return nil
		})
	}

	return sb.String(), nil
}

// extractGoSignatures parses a Go file and extracts package, func, type, and interface declarations.
func extractGoSignatures(filePath string) []string {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filePath, nil, 0)
	if err != nil {
		return nil
	}

	var sigs []string
	sigs = append(sigs, fmt.Sprintf("package %s", node.Name.Name))

	for _, decl := range node.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			sigs = append(sigs, funcSignature(d))

		case *ast.GenDecl:
			for _, spec := range d.Specs {
				if ts, ok := spec.(*ast.TypeSpec); ok {
					switch ts.Type.(type) {
					case *ast.StructType:
						sigs = append(sigs, fmt.Sprintf("type %s struct", ts.Name.Name))
					case *ast.InterfaceType:
						sigs = append(sigs, fmt.Sprintf("type %s interface", ts.Name.Name))
					default:
						sigs = append(sigs, fmt.Sprintf("type %s", ts.Name.Name))
					}
				}
			}
		}
	}

	return sigs
}

// funcSignature builds a human-readable function signature string.
func funcSignature(f *ast.FuncDecl) string {
	var sb strings.Builder
	sb.WriteString("func ")

	// Receiver
	if f.Recv != nil && len(f.Recv.List) > 0 {
		sb.WriteString("(")
		sb.WriteString(typeString(f.Recv.List[0].Type))
		sb.WriteString(") ")
	}

	sb.WriteString(f.Name.Name)
	sb.WriteString("(")

	// Parameters
	if f.Type.Params != nil {
		var params []string
		for _, p := range f.Type.Params.List {
			typeName := typeString(p.Type)
			if len(p.Names) > 0 {
				for _, name := range p.Names {
					params = append(params, fmt.Sprintf("%s %s", name.Name, typeName))
				}
			} else {
				params = append(params, typeName)
			}
		}
		sb.WriteString(strings.Join(params, ", "))
	}
	sb.WriteString(")")

	// Return types
	if f.Type.Results != nil && len(f.Type.Results.List) > 0 {
		var results []string
		for _, r := range f.Type.Results.List {
			results = append(results, typeString(r.Type))
		}
		if len(results) == 1 {
			sb.WriteString(" " + results[0])
		} else {
			sb.WriteString(" (" + strings.Join(results, ", ") + ")")
		}
	}

	return sb.String()
}

// typeString converts an ast.Expr to a readable type string.
func typeString(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + typeString(t.X)
	case *ast.SelectorExpr:
		return typeString(t.X) + "." + t.Sel.Name
	case *ast.ArrayType:
		if t.Len == nil {
			return "[]" + typeString(t.Elt)
		}
		return "[...]" + typeString(t.Elt)
	case *ast.MapType:
		return "map[" + typeString(t.Key) + "]" + typeString(t.Value)
	case *ast.InterfaceType:
		return "interface{}"
	case *ast.FuncType:
		return "func(...)"
	case *ast.Ellipsis:
		return "..." + typeString(t.Elt)
	case *ast.ChanType:
		return "chan " + typeString(t.Value)
	case *ast.StructType:
		return "struct{}"
	default:
		return "..."
	}
}
