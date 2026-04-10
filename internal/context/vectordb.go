package context

import (
	"fmt"
	"math"
	"sort"
	"eugen/internal/inference"
)

type Chunk struct {
	SourceFile string
	Text       string
	Embedding  []float64
}

type VectorStore struct {
	Chunks []Chunk
	Files  map[string]string
}

func NewVectorStore() *VectorStore {
	return &VectorStore{
		Files: make(map[string]string),
	}
}

func (vs *VectorStore) AddFile(fileName, content string) {
	vs.Files[fileName] = content
}

func (vs *VectorStore) AddChunk(sourceFile, text string, embedding []float64) {
	vs.Chunks = append(vs.Chunks, Chunk{SourceFile: sourceFile, Text: text, Embedding: embedding})
}

// cosineSimilarity computes cosine similarity between a and b.
func cosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) {
		return 0.0
	}
	var dotProduct, normA, normB float64
	for i := range a {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA == 0 || normB == 0 {
		return 0.0
	}
	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}

type docResult struct {
	fileName string
	score    float64
}

// Search evaluates all chunks and aggregates their scores (if > 0.1) by source document.
// It returns the full content of the single document with the highest aggregate score.
func (vs *VectorStore) Search(backend inference.Backend, query string, topK int, verbose bool) string {
	if len(vs.Chunks) == 0 {
		return ""
	}

	queryEmb, err := backend.Embed(query)
	if err != nil {
		if verbose {
			fmt.Printf("\n[VERBOSE] RAG Search failed to embed query: %v\n", err)
		}
		return ""
	}

	// Calculate score for each document
	docScores := make(map[string]float64)
	docMax := make(map[string]float64)
	chunkCount := make(map[string]int)

	for _, chunk := range vs.Chunks {
		score := cosineSimilarity(queryEmb, chunk.Embedding)
		if score > docMax[chunk.SourceFile] {
			docMax[chunk.SourceFile] = score
		}
		// Only sum chunks that have a meaningful similarity
		if score > 0.1 {
			docScores[chunk.SourceFile] += score
			chunkCount[chunk.SourceFile]++
		}
	}

	if len(docScores) == 0 {
		return ""
	}

	var results []docResult
	for file, score := range docScores {
		// A document is only considered if it has at least one chunk with a similarity >= 0.7
		if docMax[file] < 0.7 {
			continue
		}
		results = append(results, docResult{fileName: file, score: score})
	}

	if len(results) == 0 {
		// No document met the max >= 0.7 threshold
		return ""
	}

	// Sort descending by score
	sort.Slice(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})

	if verbose {
		fmt.Printf("\n[VERBOSE] RAG Vector Search Aggregierte Dokumenten-Scores für '%s':\n", query)
		for _, r := range results {
			fmt.Printf(" - Dokument: %s | Agggregierter Score: %.4f (Max Chunk: %.4f, %d Chunks > 0.1)\n", r.fileName, r.score, docMax[r.fileName], chunkCount[r.fileName])
		}
	}

	// Pick the winning document
	bestDoc := results[0]

	content, exists := vs.Files[bestDoc.fileName]
	if !exists {
		return ""
	}

	// Always show which document is being used (not just in verbose mode)
	fmt.Printf("\033[34m\u2139 RAG Vektorsuche: Nutze lokales Dokument '%s' (Aggregierter Score: %.2f, Max Chunk: %.2f)\033[0m\n", bestDoc.fileName, bestDoc.score, docMax[bestDoc.fileName])

	// Format result
	return fmt.Sprintf("\nZusätzliches lokales Wissen (RAG-Dokument: %s, Best-Match Relevanz: %.2f):\n%s\nBitte beachte dieses Wissen für deine Antwort.\n", bestDoc.fileName, docMax[bestDoc.fileName], content)
}
