package openai

import (
	"bufio"
	"bytes"
	"encoding/json"
	"eugen/internal/inference"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type Client struct {
	URL        string
	ApiKey     string
	Model      string
	EmbedModel string
}

func NewClient(url, apiKey, model, embedModel string) *Client {
	return &Client{
		URL:        strings.TrimRight(url, "/"),
		ApiKey:     apiKey,
		Model:      model,
		EmbedModel: embedModel,
	}
}

func (c *Client) Name() string {
	return "OpenAI-API"
}

type chatRequest struct {
	Model    string              `json:"model"`
	Messages []inference.Message `json:"messages"`
	Stream   bool                `json:"stream"`
}

type chatResponseChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
}

func (c *Client) Chat(messages []inference.Message, onToken func(string)) (string, error) {
	reqBody := chatRequest{
		Model:    c.Model,
		Messages: messages,
		Stream:   true,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", c.URL+"/chat/completions", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")
	if c.ApiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.ApiKey)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("openai error %d: %s", resp.StatusCode, string(bodyBytes))
	}

	reader := bufio.NewReader(resp.Body)
	var fullResponse strings.Builder

	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			return "", err
		}

		lineStr := strings.TrimSpace(string(line))
		if lineStr == "" {
			continue
		}

		if strings.HasPrefix(lineStr, "data: ") {
			data := strings.TrimPrefix(lineStr, "data: ")
			if data == "[DONE]" {
				break
			}

			var chunk chatResponseChunk
			if err := json.Unmarshal([]byte(data), &chunk); err == nil {
				if len(chunk.Choices) > 0 {
					content := chunk.Choices[0].Delta.Content
					fullResponse.WriteString(content)
					if onToken != nil && content != "" {
						onToken(content)
					}
				}
			}
		}
	}

	return fullResponse.String(), nil
}

func (c *Client) Generate(systemPrompt, userPrompt string, onToken func(string)) (string, error) {
	messages := []inference.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}
	return c.Chat(messages, onToken)
}

type embedRequest struct {
	Model string `json:"model"`
	Input string `json:"input"`
}

type embedResponse struct {
	Data []struct {
		Embedding []float64 `json:"embedding"`
	} `json:"data"`
}

func (c *Client) Embed(text string) ([]float64, error) {
	reqBody := embedRequest{
		Model: c.EmbedModel,
		Input: text,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", c.URL+"/embeddings", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	if c.ApiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.ApiKey)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("openai embed error %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var embResp embedResponse
	if err := json.NewDecoder(resp.Body).Decode(&embResp); err != nil {
		return nil, err
	}

	if len(embResp.Data) > 0 {
		return embResp.Data[0].Embedding, nil
	}

	return nil, fmt.Errorf("no embedding returned")
}
