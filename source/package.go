package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"ahoy"
)

// PackageFile represents a single .ahoy file in a package
type PackageFile struct {
	Path        string
	ProgramName string // Empty if standalone script
	AST         *ahoy.ASTNode
	Content     string
}

// Package represents a collection of files with the same program name
type Package struct {
	Name  string
	Files []PackageFile
}

// PackageManager handles package resolution and compilation
type PackageManager struct {
	Packages      map[string]*Package     // program name -> Package
	ImportedPaths map[string]*Package     // file/dir path -> Package
	CurrentDir    string
}

func NewPackageManager(currentDir string) *PackageManager {
	return &PackageManager{
		Packages:      make(map[string]*Package),
		ImportedPaths: make(map[string]*Package),
		CurrentDir:    currentDir,
	}
}

// LoadFile loads and parses a .ahoy file
func (pm *PackageManager) LoadFile(filePath string) (*PackageFile, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("error reading file %s: %v", filePath, err)
	}

	// TEMP: Disable formatter for debugging
	formattedContent := string(content) // formatSource(string(content))
	tokens := ahoy.Tokenize(formattedContent)
	ast := ahoy.Parse(tokens)

	pf := &PackageFile{
		Path:    filePath,
		AST:     ast,
		Content: formattedContent,
	}

	// Check if first statement is a program declaration
	if ast != nil && len(ast.Children) > 0 {
		firstNode := ast.Children[0]
		if firstNode.Type == ahoy.NODE_PROGRAM_DECLARATION {
			pf.ProgramName = firstNode.Value
		}
	}

	return pf, nil
}

// LoadPackageFromFile loads a file and its associated package files
func (pm *PackageManager) LoadPackageFromFile(mainFilePath string) (*Package, error) {
	// Load the main file
	mainFile, err := pm.LoadFile(mainFilePath)
	if err != nil {
		return nil, err
	}

	// If no program declaration, return single-file package
	if mainFile.ProgramName == "" {
		pkg := &Package{
			Name:  filepath.Base(mainFilePath),
			Files: []PackageFile{*mainFile},
		}
		return pkg, nil
	}

	// Find all files in the same directory with the same program name
	dir := filepath.Dir(mainFilePath)
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("error reading directory %s: %v", dir, err)
	}

	pkg := &Package{
		Name:  mainFile.ProgramName,
		Files: []PackageFile{},
	}

	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".ahoy") {
			continue
		}

		filePath := filepath.Join(dir, file.Name())
		pf, err := pm.LoadFile(filePath)
		if err != nil {
			return nil, err
		}

		// Only include files with matching program name
		if pf.ProgramName == mainFile.ProgramName {
			pkg.Files = append(pkg.Files, *pf)
		}
	}

	pm.Packages[pkg.Name] = pkg
	return pkg, nil
}

// ResolveImport resolves an import path to a Package
func (pm *PackageManager) ResolveImport(importPath string, fromFile string) (*Package, error) {
	// Check if already imported
	if pkg, exists := pm.ImportedPaths[importPath]; exists {
		return pkg, nil
	}

	// Resolve relative paths
	var resolvedPath string
	if strings.HasPrefix(importPath, "./") || strings.HasPrefix(importPath, "../") {
		baseDir := filepath.Dir(fromFile)
		resolvedPath = filepath.Join(baseDir, importPath)
	} else if filepath.IsAbs(importPath) {
		resolvedPath = importPath
	} else {
		// Try relative to current directory
		resolvedPath = filepath.Join(pm.CurrentDir, importPath)
	}

	// Check if path is a directory or file
	info, err := os.Stat(resolvedPath)
	if err != nil {
		return nil, fmt.Errorf("import path not found: %s", importPath)
	}

	var pkg *Package
	if info.IsDir() {
		// Load all .ahoy files in directory
		pkg, err = pm.LoadPackageFromDirectory(resolvedPath)
	} else if strings.HasSuffix(resolvedPath, ".ahoy") {
		// Load single file or package starting from this file
		pkg, err = pm.LoadPackageFromFile(resolvedPath)
	} else {
		return nil, fmt.Errorf("import path must be a directory or .ahoy file: %s", importPath)
	}

	if err != nil {
		return nil, err
	}

	pm.ImportedPaths[importPath] = pkg
	return pkg, nil
}

// LoadPackageFromDirectory loads all .ahoy files in a directory
// If they have the same program declaration, they're grouped together
func (pm *PackageManager) LoadPackageFromDirectory(dirPath string) (*Package, error) {
	files, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, fmt.Errorf("error reading directory %s: %v", dirPath, err)
	}

	packageFiles := make(map[string][]PackageFile) // program name -> files
	standaloneFiles := []PackageFile{}

	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".ahoy") {
			continue
		}

		filePath := filepath.Join(dirPath, file.Name())
		pf, err := pm.LoadFile(filePath)
		if err != nil {
			return nil, err
		}

		if pf.ProgramName == "" {
			standaloneFiles = append(standaloneFiles, *pf)
		} else {
			packageFiles[pf.ProgramName] = append(packageFiles[pf.ProgramName], *pf)
		}
	}

	// If there's only one program group, return that
	if len(packageFiles) == 1 {
		for name, files := range packageFiles {
			pkg := &Package{
				Name:  name,
				Files: files,
			}
			pm.Packages[name] = pkg
			return pkg, nil
		}
	}

	// If multiple programs or standalone files, return error
	if len(packageFiles) > 1 {
		names := make([]string, 0, len(packageFiles))
		for name := range packageFiles {
			names = append(names, name)
		}
		return nil, fmt.Errorf("directory contains multiple programs: %v", names)
	}

	// Only standalone files
	if len(standaloneFiles) > 0 {
		return nil, fmt.Errorf("directory contains only standalone files without program declarations")
	}

	return nil, fmt.Errorf("no .ahoy files found in directory: %s", dirPath)
}

// GetAllFunctions returns all function declarations from a package
func (pkg *Package) GetAllFunctions() []*ahoy.ASTNode {
	functions := []*ahoy.ASTNode{}
	for _, file := range pkg.Files {
		if file.AST != nil {
			for _, child := range file.AST.Children {
				if child.Type == ahoy.NODE_FUNCTION {
					functions = append(functions, child)
				}
			}
		}
	}
	return functions
}

// GetAllGlobalVariables returns all global variable declarations from a package
func (pkg *Package) GetAllGlobalVariables() []*ahoy.ASTNode {
	variables := []*ahoy.ASTNode{}
	for _, file := range pkg.Files {
		if file.AST != nil {
			for _, child := range file.AST.Children {
				if child.Type == ahoy.NODE_VARIABLE_DECLARATION || 
				   child.Type == ahoy.NODE_CONSTANT_DECLARATION {
					variables = append(variables, child)
				}
			}
		}
	}
	return variables
}

// GetAllStructs returns all struct declarations from a package
func (pkg *Package) GetAllStructs() []*ahoy.ASTNode {
	structs := []*ahoy.ASTNode{}
	for _, file := range pkg.Files {
		if file.AST != nil {
			for _, child := range file.AST.Children {
				if child.Type == ahoy.NODE_STRUCT_DECLARATION {
					structs = append(structs, child)
				}
			}
		}
	}
	return structs
}

// GetAllEnums returns all enum declarations from a package
func (pkg *Package) GetAllEnums() []*ahoy.ASTNode {
	enums := []*ahoy.ASTNode{}
	for _, file := range pkg.Files {
		if file.AST != nil {
			for _, child := range file.AST.Children {
				if child.Type == ahoy.NODE_ENUM_DECLARATION {
					enums = append(enums, child)
				}
			}
		}
	}
	return enums
}

// MergeAST creates a single AST from all package files, deduplicating imports
func (pkg *Package) MergeAST() *ahoy.ASTNode {
	merged := &ahoy.ASTNode{Type: ahoy.NODE_PROGRAM}
	seenImports := make(map[string]bool)

	for _, file := range pkg.Files {
		if file.AST != nil {
			for _, child := range file.AST.Children {
				// Skip program declarations in merged output
				if child.Type == ahoy.NODE_PROGRAM_DECLARATION {
					continue
				}
				
				// Deduplicate imports
				if child.Type == ahoy.NODE_IMPORT_STATEMENT {
					importKey := child.Value + "|" + child.DataType // path + namespace
					if seenImports[importKey] {
						continue
					}
					seenImports[importKey] = true
				}
				
				merged.Children = append(merged.Children, child)
			}
		}
	}

	return merged
}
