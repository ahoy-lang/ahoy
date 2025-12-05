package main

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sync"
)

//go:embed tcc
var embeddedTCC embed.FS

var (
	tccExtractDir  string
	tccExtractOnce sync.Once
	tccExtractErr  error
)

// getTCCDir returns the directory where TCC is extracted
// Uses a consistent location in user's cache directory so it persists
func getTCCDir() (string, error) {
	tccExtractOnce.Do(func() {
		// Use user cache directory for persistent storage
		cacheDir, err := os.UserCacheDir()
		if err != nil {
			// Fall back to home directory
			homeDir, err := os.UserHomeDir()
			if err != nil {
				tccExtractErr = fmt.Errorf("cannot find cache or home directory: %w", err)
				return
			}
			cacheDir = filepath.Join(homeDir, ".cache")
		}

		// Create ahoy-specific cache directory
		tccExtractDir = filepath.Join(cacheDir, "ahoy", "tcc")

		// Check if already extracted and valid
		if isTCCValid(tccExtractDir) {
			return
		}

		// Extract TCC files
		err = os.MkdirAll(tccExtractDir, 0755)
		if err != nil {
			tccExtractErr = fmt.Errorf("failed to create TCC directory: %w", err)
			return
		}

		err = extractTCC(embeddedTCC, tccExtractDir)
		if err != nil {
			tccExtractErr = fmt.Errorf("failed to extract TCC: %w", err)
			return
		}
	})
	return tccExtractDir, tccExtractErr
}

// isTCCValid checks if TCC is properly extracted
func isTCCValid(dir string) bool {
	switch runtime.GOOS {
	case "linux":
		tccBin := filepath.Join(dir, "linux", "tcc")
		libtcc := filepath.Join(dir, "linux", "libtcc1.a")
		if _, err := os.Stat(tccBin); err != nil {
			return false
		}
		if _, err := os.Stat(libtcc); err != nil {
			return false
		}
		return true
	case "windows":
		tccBin := filepath.Join(dir, "windows", "tcc.exe")
		if _, err := os.Stat(tccBin); err != nil {
			return false
		}
		return true
	default:
		return false
	}
}

// extractTCC extracts embedded TCC files to target directory
func extractTCC(efs embed.FS, targetDir string) error {
	return fs.WalkDir(efs, "tcc", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Remove the leading "tcc/" from the path since it's already part of targetDir structure
		// e.g., "tcc/linux/tcc" becomes "linux/tcc"
		relPath := path
		if len(path) > 4 && path[:4] == "tcc/" {
			relPath = path[4:]
		} else if path == "tcc" {
			// Skip the root tcc directory itself
			return nil
		}
		
		targetPath := filepath.Join(targetDir, relPath)

		if d.IsDir() {
			return os.MkdirAll(targetPath, 0755)
		}

		// Read file from embedded FS
		data, err := efs.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read embedded file %s: %w", path, err)
		}

		// Write to target
		err = os.WriteFile(targetPath, data, 0644)
		if err != nil {
			return fmt.Errorf("failed to write file %s: %w", targetPath, err)
		}

		// Make executables executable
		if filepath.Base(path) == "tcc" || filepath.Ext(path) == ".exe" {
			os.Chmod(targetPath, 0755)
		}

		return nil
	})
}

// GetEmbeddedTCCPath returns the path to the embedded TCC compiler and its args
func GetEmbeddedTCCPath() (string, []string, error) {
	tccDir, err := getTCCDir()
	if err != nil {
		return "", nil, err
	}

	switch runtime.GOOS {
	case "linux":
		tccPath := filepath.Join(tccDir, "linux", "tcc")
		tccArgs := []string{"-B" + filepath.Join(tccDir, "linux")}
		return tccPath, tccArgs, nil

	case "windows":
		tccPath := filepath.Join(tccDir, "windows", "tcc.exe")
		tccArgs := []string{"-B" + filepath.Join(tccDir, "windows")}
		return tccPath, tccArgs, nil

	default:
		return "", nil, fmt.Errorf("embedded TCC not available for %s", runtime.GOOS)
	}
}
