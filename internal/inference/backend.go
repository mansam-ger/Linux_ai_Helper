package inference

// Message represents a single chat message with a role (system, user, assistant).
// This is the common message type shared across all inference backends.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Backend is the common interface that all inference backends must implement.
// This allows Eugen to switch between different LLM providers (Ollama, OpenAI, vLLM, etc.)
// without changing any business logic.
type Backend interface {
	// Generate sends a one-shot prompt (system + user) and returns the full response.
	// The onToken callback is called for each streamed token for live output.
	// Used by planner, validator, and diagnostic analysis.
	Generate(systemPrompt, userPrompt string, onToken func(string)) (string, error)

	// Chat sends a full message history and returns the assistant's response.
	// The onToken callback is called for each streamed token for live output.
	// Used for the interactive REPL conversation with memory.
	Chat(messages []Message, onToken func(string)) (string, error)

	// Embed generates a vector representation of the provided text.
	Embed(text string) ([]float64, error)

	// Name returns a human-readable name for this backend (e.g. "Ollama", "OpenAI").
	Name() string
}
