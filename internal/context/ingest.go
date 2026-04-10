package context

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"eugen/internal/config"
	"eugen/internal/inference"
)

const MaxIngestSize = 100000 // roughly 100KB

// ingestExclude lists filenames that should NOT be treated as RAG documents.
var ingestExclude = map[string]bool{
	"eugen_db.json": true,
	"eugen.conf":    true,
}

// BuildVectorDatabase reads .txt and .md files from the data directory, chunks them,
// embeds them via the backend, and returns a populated VectorStore.
func BuildVectorDatabase(backend inference.Backend, verbose bool) *VectorStore {
	store := NewVectorStore()
	dir := config.DataDir

	info, err := os.Stat(dir)
	if os.IsNotExist(err) || !info.IsDir() {
		os.MkdirAll(dir, 0755)
		return store
	}

	files, err := os.ReadDir(dir)
	if err != nil {
		return store
	}

	for _, d := range files {
		if d.IsDir() {
			continue
		}

		if ingestExclude[d.Name()] {
			continue
		}

		ext := strings.ToLower(filepath.Ext(d.Name()))
		if ext == ".txt" || ext == ".md" {
			path := filepath.Join(dir, d.Name())
			content, err := os.ReadFile(path)
			if err != nil {
				continue
			}

			contentStr := strings.ToValidUTF8(string(content), "")
			store.AddFile(d.Name(), contentStr)
			
			// Simple chunking by double newlines (paragraphs)
			paragraphs := strings.Split(contentStr, "\n\n")
			var currentChunk strings.Builder
			
			for _, p := range paragraphs {
				p = strings.TrimSpace(p)
				if p == "" {
					continue
				}
				
				if currentChunk.Len()+len(p) > 1000 {
					// Embed current chunk
					chunkText := currentChunk.String()
					emb, err := backend.Embed(chunkText)
					if err == nil {
						store.AddChunk(d.Name(), chunkText, emb)
					} else {
						fmt.Printf("\u26A0 Fehler bei RAG Embedding von %s: %v\n", d.Name(), err)
					}
					currentChunk.Reset()
				}
				
				if currentChunk.Len() > 0 {
					currentChunk.WriteString("\n\n")
				}
				currentChunk.WriteString(p)
			}
			
			// Embed remaining
			if currentChunk.Len() > 0 {
				chunkText := currentChunk.String()
				emb, err := backend.Embed(chunkText)
				if err == nil {
					store.AddChunk(d.Name(), chunkText, emb)
				} else {
					fmt.Printf("\u26A0 Fehler bei RAG Embedding von %s: %v\n", d.Name(), err)
				}
			}
		}
	}

	return store
}
