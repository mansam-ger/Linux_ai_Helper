package context

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
)

type CachedFile struct {
	Hash   string
	Chunks []Chunk
}

type RagCache struct {
	Files map[string]CachedFile
}

func NewRagCache() *RagCache {
	return &RagCache{
		Files: make(map[string]CachedFile),
	}
}

func HashContent(content string) string {
	h := sha256.New()
	h.Write([]byte(content))
	return fmt.Sprintf("%x", h.Sum(nil))
}

func LoadCache(path string) *RagCache {
	cache := NewRagCache()
	
	data, err := os.ReadFile(path)
	if err != nil {
		return cache
	}
	
	err = json.Unmarshal(data, cache)
	if err != nil {
		// If the cache is corrupt, start fresh
		return NewRagCache()
	}
	
	// Ensure the map is initialized in case JSON was weird
	if cache.Files == nil {
		cache.Files = make(map[string]CachedFile)
	}
	
	return cache
}

func SaveCache(path string, cache *RagCache) error {
	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
