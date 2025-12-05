package main

import (
	"crypto/md5"
	"encoding/gob"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"ahoy"
)

// FileCache stores cached parsing results for a single file
type FileCache struct {
	Path        string
	ModTime     time.Time
	Size        int64
	Checksum    string
	AST         *ahoy.ASTNode
	Content     string
	ProgramName string
}

// CHeaderCache stores cached C header parsing results
type CHeaderCache struct {
	Path     string
	ModTime  time.Time
	Size     int64
	Checksum string
	Info     *ahoy.CHeaderInfo
}

// BuildCache manages the incremental build cache
type BuildCache struct {
	CacheDir    string
	ProgramName string                    // Name of the program being cached
	Files       map[string]*FileCache    // path -> cache entry
	CHeaders    map[string]*CHeaderCache // path -> C header cache entry
	Enabled     bool
	CacheTime   time.Duration // Total time spent on cache operations
	CacheHits   int           // Number of cache hits
	CacheMisses int           // Number of cache misses
	mu          sync.RWMutex  // Protects concurrent access to cache
}

// NewBuildCache creates a new build cache
func NewBuildCache(enabled bool) *BuildCache {
	cacheDir := getCacheDir()

	bc := &BuildCache{
		CacheDir: cacheDir,
		Files:    make(map[string]*FileCache),
		CHeaders: make(map[string]*CHeaderCache),
		Enabled:  enabled,
	}

	// Don't load here - wait until SetProgramName is called
	return bc
}

// SetProgramName sets the program name and loads the program-specific cache
func (bc *BuildCache) SetProgramName(name string) {
	if name == "" {
		name = "default"
	}
	bc.ProgramName = name
	if bc.Enabled {
		bc.load()
	}
}

// getCacheDir returns the cache directory path
func getCacheDir() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ".ahoy_cache"
	}
	return filepath.Join(homeDir, ".cache", "ahoy", "build")
}

// CacheData is the structure saved to disk
type CacheData struct {
	Files    map[string]*FileCache
	CHeaders map[string]*CHeaderCache
}

// getCacheFileName returns the cache file name for the current program
func (bc *BuildCache) getCacheFileName() string {
	name := bc.ProgramName
	if name == "" {
		name = "default"
	}
	return name + "_cache.gob"
}

// load loads the cache from disk
func (bc *BuildCache) load() {
	if !bc.Enabled {
		return
	}

	cacheFile := filepath.Join(bc.CacheDir, bc.getCacheFileName())
	file, err := os.Open(cacheFile)
	if err != nil {
		// Cache doesn't exist yet, that's fine
		return
	}
	defer file.Close()

	var data CacheData
	decoder := gob.NewDecoder(file)
	if err := decoder.Decode(&data); err != nil {
		// Cache corrupted, clear it
		bc.Files = make(map[string]*FileCache)
		bc.CHeaders = make(map[string]*CHeaderCache)
		return
	}

	if data.Files != nil {
		bc.Files = data.Files
	}
	if data.CHeaders != nil {
		bc.CHeaders = data.CHeaders
	}
}

// save saves the cache to disk
func (bc *BuildCache) save() {
	if !bc.Enabled {
		return
	}

	// Create cache directory if needed
	if err := os.MkdirAll(bc.CacheDir, 0755); err != nil {
		return
	}

	cacheFile := filepath.Join(bc.CacheDir, bc.getCacheFileName())
	file, err := os.Create(cacheFile)
	if err != nil {
		return
	}
	defer file.Close()

	data := CacheData{
		Files:    bc.Files,
		CHeaders: bc.CHeaders,
	}
	encoder := gob.NewEncoder(file)
	encoder.Encode(data)
}

// computeChecksum computes MD5 checksum of content
func computeChecksum(content string) string {
	hash := md5.Sum([]byte(content))
	return hex.EncodeToString(hash[:])
}

// IsFileChanged checks if a file has changed since it was cached
func (bc *BuildCache) IsFileChanged(filePath string) bool {
	if !bc.Enabled {
		return true
	}
	
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return true
	}
	
	cached, exists := bc.Files[absPath]
	if !exists {
		return true
	}
	
	// Get current file info
	info, err := os.Stat(absPath)
	if err != nil {
		return true
	}
	
	// Quick check: mod time and size
	if info.ModTime() != cached.ModTime || info.Size() != cached.Size {
		return true
	}
	
	return false
}

// GetCachedFile returns cached parsing result if valid
func (bc *BuildCache) GetCachedFile(filePath string) (*PackageFile, bool) {
	start := time.Now()
	defer func() {
		bc.mu.Lock()
		bc.CacheTime += time.Since(start)
		bc.mu.Unlock()
	}()

	if !bc.Enabled {
		return nil, false
	}

	absPath, err := filepath.Abs(filePath)
	if err != nil {
		bc.mu.Lock()
		bc.CacheMisses++
		bc.mu.Unlock()
		return nil, false
	}

	if bc.IsFileChanged(absPath) {
		bc.mu.Lock()
		bc.CacheMisses++
		bc.mu.Unlock()
		return nil, false
	}

	bc.mu.RLock()
	cached := bc.Files[absPath]
	bc.mu.RUnlock()
	
	if cached == nil || cached.AST == nil {
		bc.mu.Lock()
		bc.CacheMisses++
		bc.mu.Unlock()
		return nil, false
	}

	// Verify checksum by reading current file content
	content, err := os.ReadFile(absPath)
	if err != nil {
		bc.mu.Lock()
		bc.CacheMisses++
		bc.mu.Unlock()
		return nil, false
	}

	currentChecksum := computeChecksum(string(content))
	if currentChecksum != cached.Checksum {
		bc.mu.Lock()
		bc.CacheMisses++
		bc.mu.Unlock()
		return nil, false
	}

	bc.mu.Lock()
	bc.CacheHits++
	bc.mu.Unlock()

	return &PackageFile{
		Path:        cached.Path,
		ProgramName: cached.ProgramName,
		AST:         cached.AST,
		Content:     cached.Content,
	}, true
}

// CacheFile stores a parsed file in the cache
func (bc *BuildCache) CacheFile(pf *PackageFile) {
	if !bc.Enabled || pf == nil {
		return
	}

	absPath, err := filepath.Abs(pf.Path)
	if err != nil {
		return
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return
	}

	bc.mu.Lock()
	bc.Files[absPath] = &FileCache{
		Path:        absPath,
		ModTime:     info.ModTime(),
		Size:        info.Size(),
		Checksum:    computeChecksum(pf.Content),
		AST:         pf.AST,
		Content:     pf.Content,
		ProgramName: pf.ProgramName,
	}
	bc.mu.Unlock()
}

// GetCachedCHeader returns cached C header parsing result if valid
func (bc *BuildCache) GetCachedCHeader(headerPath string) (*ahoy.CHeaderInfo, bool) {
	start := time.Now()
	defer func() {
		bc.mu.Lock()
		bc.CacheTime += time.Since(start)
		bc.mu.Unlock()
	}()

	if !bc.Enabled {
		return nil, false
	}

	absPath, err := filepath.Abs(headerPath)
	if err != nil {
		return nil, false
	}

	bc.mu.RLock()
	cached, exists := bc.CHeaders[absPath]
	bc.mu.RUnlock()

	if !exists || cached == nil || cached.Info == nil {
		return nil, false
	}

	// Get current file info
	info, err := os.Stat(absPath)
	if err != nil {
		return nil, false
	}

	// Quick check: mod time and size
	if info.ModTime() != cached.ModTime || info.Size() != cached.Size {
		return nil, false
	}

	bc.mu.Lock()
	bc.CacheHits++
	bc.mu.Unlock()

	return cached.Info, true
}

// CacheCHeader stores a parsed C header in the cache
func (bc *BuildCache) CacheCHeader(headerPath string, info *ahoy.CHeaderInfo) {
	if !bc.Enabled || info == nil {
		return
	}

	absPath, err := filepath.Abs(headerPath)
	if err != nil {
		return
	}

	fileInfo, err := os.Stat(absPath)
	if err != nil {
		return
	}

	// Read file content for checksum
	content, err := os.ReadFile(absPath)
	if err != nil {
		return
	}

	bc.mu.Lock()
	bc.CHeaders[absPath] = &CHeaderCache{
		Path:     absPath,
		ModTime:  fileInfo.ModTime(),
		Size:     fileInfo.Size(),
		Checksum: computeChecksum(string(content)),
		Info:     info,
	}
	bc.mu.Unlock()
}

// GetCacheTime returns the total time spent on cache operations
func (bc *BuildCache) GetCacheTime() time.Duration {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	return bc.CacheTime
}

// GetCacheStats returns cache hit/miss statistics
func (bc *BuildCache) GetCacheStats() (hits, misses int) {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	return bc.CacheHits, bc.CacheMisses
}

// SaveAndClose saves the cache and cleans up
func (bc *BuildCache) SaveAndClose() {
	bc.save()
}

// Clear clears all cached data
func (bc *BuildCache) Clear() {
	bc.mu.Lock()
	bc.Files = make(map[string]*FileCache)
	bc.CHeaders = make(map[string]*CHeaderCache)
	bc.mu.Unlock()
	if bc.CacheDir != "" {
		cacheFile := filepath.Join(bc.CacheDir, "cache.gob")
		os.Remove(cacheFile)
	}
}

// PrintStats prints cache statistics
func (bc *BuildCache) PrintStats() {
	if !bc.Enabled {
		fmt.Println("Incremental build cache: disabled")
		return
	}

	bc.mu.RLock()
	defer bc.mu.RUnlock()
	fmt.Printf("Incremental build cache: %d files, %d headers cached\n", len(bc.Files), len(bc.CHeaders))
}

// Global cache instance
var buildCache *BuildCache

// InitBuildCache initializes the global build cache
func InitBuildCache(enabled bool) {
	buildCache = NewBuildCache(enabled)
}

// GetBuildCache returns the global build cache
func GetBuildCache() *BuildCache {
	if buildCache == nil {
		buildCache = NewBuildCache(false)
	}
	return buildCache
}
