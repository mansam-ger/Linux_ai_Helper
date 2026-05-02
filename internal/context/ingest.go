package context

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"eugen/internal/config"
	"eugen/internal/inference"
)

const MaxIngestSize = 100000 // roughly 100KB

// ingestExclude lists filenames that should NOT be treated as RAG documents.
var ingestExclude = map[string]bool{
	"eugen_db.json":  true,
	"eugen.conf":     true,
	"rag_cache.json": true,
}

// BuildVectorDatabase reads .txt and .md files from the data directory, chunks them,
// embeds them via the backend, and returns a populated VectorStore.
func BuildVectorDatabase(backend inference.Backend, verbose bool) *VectorStore {
	store := NewVectorStore()
	dir := config.GetDataDir()
	
	cachePath := filepath.Join(dir, "rag_cache.json")
	ragCache := LoadCache(cachePath)
	cacheUpdated := false

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
			
			hash := HashContent(contentStr)
			
			// Check Cache
			if cached, ok := ragCache.Files[d.Name()]; ok && cached.Hash == hash {
				// Cache Hit
				fmt.Printf("[\u2139] RAG Cache HIT für '%s'. Lade %d Chunks...\n", d.Name(), len(cached.Chunks))
				for _, c := range cached.Chunks {
					store.AddChunk(c.SourceFile, c.Text, c.Embedding)
				}
			} else {
				// Cache Miss
				fmt.Printf("[\u23F3] RAG Cache MISS für '%s'. Berechne Embeddings (das kann einen Moment dauern)...\n", d.Name())
				
				startLen := len(store.Chunks)
				ingestContent(store, backend, d.Name(), contentStr)
				
				// Keep a copy of the newly generated chunks for caching
				var newChunks []Chunk
				if len(store.Chunks) > startLen {
					newChunks = make([]Chunk, len(store.Chunks)-startLen)
					copy(newChunks, store.Chunks[startLen:])
				}
				
				ragCache.Files[d.Name()] = CachedFile{
					Hash:   hash,
					Chunks: newChunks,
				}
				cacheUpdated = true
			}
		}
	}

	if cacheUpdated {
		err := SaveCache(cachePath, ragCache)
		if err != nil && verbose {
			fmt.Printf("\u26A0 Fehler beim Speichern des RAG Caches: %v\n", err)
		}
	}

	return store
}

func ingestContent(store *VectorStore, backend inference.Backend, sourceName, contentStr string) {
	paragraphs := strings.Split(contentStr, "\n\n")
	var currentChunk strings.Builder
	
	for _, p := range paragraphs {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		
		// If the paragraph itself is too large, we must split it forcefully
		if len(p) > 1000 {
			// If we have something in currentChunk, embed it first
			if currentChunk.Len() > 0 {
				chunkText := currentChunk.String()
				emb, err := backend.Embed(chunkText)
				if err == nil {
					store.AddChunk(sourceName, chunkText, emb)
				} else {
					fmt.Printf("\u26A0 Fehler bei RAG Embedding von %s: %v\n", sourceName, err)
				}
				currentChunk.Reset()
			}
			
			// Hard chunk the large paragraph
			for len(p) > 1000 {
				chunkText := p[:1000]
				emb, err := backend.Embed(chunkText)
				if err == nil {
					store.AddChunk(sourceName, chunkText, emb)
				} else {
					fmt.Printf("\u26A0 Fehler bei RAG Embedding von %s: %v\n", sourceName, err)
				}
				p = p[1000:]
			}
			// Remaining part of p goes into currentChunk
			currentChunk.WriteString(p)
			continue
		}

		if currentChunk.Len()+len(p) > 1000 {
			chunkText := currentChunk.String()
			emb, err := backend.Embed(chunkText)
			if err == nil {
				store.AddChunk(sourceName, chunkText, emb)
			} else {
				fmt.Printf("\u26A0 Fehler bei RAG Embedding von %s: %v\n", sourceName, err)
			}
			currentChunk.Reset()
		}
		
		if currentChunk.Len() > 0 {
			currentChunk.WriteString("\n\n")
		}
		currentChunk.WriteString(p)
	}
	
	if currentChunk.Len() > 0 {
		chunkText := currentChunk.String()
		emb, err := backend.Embed(chunkText)
		if err == nil {
			store.AddChunk(sourceName, chunkText, emb)
		} else {
			fmt.Printf("\u26A0 Fehler bei RAG Embedding von %s: %v\n", sourceName, err)
		}
	}
}

// IngestManPage retrieves the man page for a given tool, saves it persistently in eugen_data,
// and adds it to the current VectorStore.
func IngestManPage(store *VectorStore, backend inference.Backend, toolName string) error {
	cmdStr := fmt.Sprintf("man %s | col -bx", toolName)
	out, err := exec.Command("sh", "-c", cmdStr).CombinedOutput()
	if err != nil {
		return fmt.Errorf("konnte man-page für '%s' nicht lesen: %v", toolName, err)
	}
	
	outputStr := string(out)
	if strings.Contains(outputStr, "No manual entry") {
		return fmt.Errorf("keine Man-Page für '%s' gefunden", toolName)
	}
	
	// Validate length to ensure it's not empty or just a few chars
	if len(strings.TrimSpace(outputStr)) < 50 {
		return fmt.Errorf("ausgelesene Man-Page für '%s' ist zu kurz oder leer", toolName)
	}

	contentStr := strings.ToValidUTF8(outputStr, "")

	// Save to eugen_data so it persists for future sessions
	fileName := fmt.Sprintf("man_%s.txt", toolName)
	filePath := filepath.Join(config.GetDataDir(), fileName)
	err = os.WriteFile(filePath, []byte(contentStr), 0644)
	if err != nil {
		fmt.Printf("\u26A0 Konnte '%s' nicht persistent speichern: %v\n", fileName, err)
	}

	// Add to live store
	store.AddFile(fileName, contentStr)
	
	cachePath := filepath.Join(config.GetDataDir(), "rag_cache.json")
	ragCache := LoadCache(cachePath)
	hash := HashContent(contentStr)
	
	startLen := len(store.Chunks)
	ingestContent(store, backend, fileName, contentStr)
	
	var newChunks []Chunk
	if len(store.Chunks) > startLen {
		newChunks = make([]Chunk, len(store.Chunks)-startLen)
		copy(newChunks, store.Chunks[startLen:])
	}
	
	ragCache.Files[fileName] = CachedFile{
		Hash:   hash,
		Chunks: newChunks,
	}
	_ = SaveCache(cachePath, ragCache)
	
	return nil
}
