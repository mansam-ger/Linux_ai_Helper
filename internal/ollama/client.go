package ollama

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"eugen/internal/inference"
)

// Client handles interaction with the local Ollama instance.
// Implements the inference.Backend interface.
type Client struct {
	BaseURL    string
	Model      string
	EmbedModel string
	Client     *http.Client
}

// NewClient creates a new Ollama client.
func NewClient(baseURL, model, embedModel string) *Client {
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	if model == "" {
		model = "nemotron-cascade-2:latest"
	}
	if embedModel == "" {
		embedModel = "nomic-embed-text"
	}
	return &Client{
		BaseURL:    baseURL,
		Model:      model,
		EmbedModel: embedModel,
		Client: &http.Client{
			// Generous timeout for local generation (cold-start model loading + large log analysis)
			Timeout: 10 * time.Minute,
		},
	}
}

// Name returns the human-readable backend name.
func (c *Client) Name() string {
	return "Ollama"
}

// GenerateRequest represents the body sent to the Ollama /api/generate API.
type GenerateRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	System string `json:"system,omitempty"`
	Stream bool   `json:"stream"`
}

// GenerateResponse represents the /api/generate single response.
type GenerateResponse struct {
	Response string `json:"response"`
	Done     bool   `json:"done"`
}

// ChatRequest represents the body sent to the Ollama /api/chat API.
type ChatRequest struct {
	Model    string              `json:"model"`
	Messages []inference.Message `json:"messages"`
	Stream   bool                `json:"stream"`
}

// ChatResponse represents the /api/chat response.
type ChatResponse struct {
	Message inference.Message `json:"message"`
	Done    bool              `json:"done"`
}

// EmbedRequest represents the body sent to the Ollama /api/embeddings API.
type EmbedRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
}

// EmbedResponse represents the /api/embeddings response.
type EmbedResponse struct {
	Embedding []float64 `json:"embedding"`
}

// Embed generates a vector representation of the provided text.
func (c *Client) Embed(text string) ([]float64, error) {
	reqBody := EmbedRequest{
		Model:  c.EmbedModel,
		Prompt: text,
	}

	b, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/api/embeddings", c.BaseURL)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to contact Ollama for embeddings at %s: %w", c.BaseURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama embeddings API error (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var embResp EmbedResponse
	if err := json.NewDecoder(resp.Body).Decode(&embResp); err != nil {
		return nil, fmt.Errorf("failed to decode embeddings response: %w", err)
	}

	return embResp.Embedding, nil
}

// Generate sends a stateless prompt to Ollama (/api/generate).
// Used by validator, planner, and other one-shot calls without history.
func (c *Client) Generate(systemPrompt, userPrompt string, onToken func(string)) (string, error) {
	reqBody := GenerateRequest{
		Model:  c.Model,
		Prompt: userPrompt,
		System: systemPrompt,
		Stream: true,
	}

	b, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf("%s/api/generate", c.BaseURL)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(b))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.Client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to contact Ollama at %s: %w", c.BaseURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("ollama API error (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	decoder := json.NewDecoder(resp.Body)
	var fullResponse strings.Builder

	for {
		var genResp GenerateResponse
		if err := decoder.Decode(&genResp); err != nil {
			if err == io.EOF {
				break
			}
			return fullResponse.String(), err
		}

		if onToken != nil && genResp.Response != "" {
			onToken(genResp.Response)
		}
		fullResponse.WriteString(genResp.Response)

		if genResp.Done {
			break
		}
	}

	return fullResponse.String(), nil
}

// Chat sends a full message history to Ollama (/api/chat) and returns the
// assistant's response. This is the core of the conversation memory.
func (c *Client) Chat(messages []inference.Message, onToken func(string)) (string, error) {
	reqBody := ChatRequest{
		Model:    c.Model,
		Messages: messages,
		Stream:   true,
	}

	b, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf("%s/api/chat", c.BaseURL)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(b))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.Client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to contact Ollama at %s: %w", c.BaseURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("ollama API error (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	decoder := json.NewDecoder(resp.Body)
	var fullResponse strings.Builder

	for {
		var chatResp ChatResponse
		if err := decoder.Decode(&chatResp); err != nil {
			if err == io.EOF {
				break
			}
			return fullResponse.String(), err
		}

		if onToken != nil && chatResp.Message.Content != "" {
			onToken(chatResp.Message.Content)
		}
		fullResponse.WriteString(chatResp.Message.Content)

		if chatResp.Done {
			break
		}
	}

	return fullResponse.String(), nil
}
