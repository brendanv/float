package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const baseURL = "https://openrouter.ai/api/v1/chat/completions"

// Client calls the OpenRouter chat completions API (OpenAI-compatible).
type Client struct {
	apiKey     string
	model      string
	httpClient *http.Client
}

// ChatMessage is one turn in a conversation.
type ChatMessage struct {
	Role    string `json:"role"`    // "system" | "user" | "assistant"
	Content string `json:"content"`
}

// responseFormat instructs the model to return structured JSON.
type responseFormat struct {
	Type       string     `json:"type"` // "json_schema"
	JSONSchema jsonSchema `json:"json_schema"`
}

type jsonSchema struct {
	Name   string         `json:"name"`
	Strict bool           `json:"strict"`
	Schema map[string]any `json:"schema"`
}

func NewClient(apiKey, model string) *Client {
	return &Client{
		apiKey:     apiKey,
		model:      model,
		httpClient: &http.Client{Timeout: 60 * time.Second},
	}
}

// chat sends messages with a structured output schema and returns the raw JSON
// content from the first choice. The response is guaranteed to match schema.
func (c *Client) chat(ctx context.Context, messages []ChatMessage, schemaName string, schema map[string]any) (string, error) {
	body, err := json.Marshal(map[string]any{
		"model":    c.model,
		"messages": messages,
		"response_format": responseFormat{
			Type: "json_schema",
			JSONSchema: jsonSchema{
				Name:   schemaName,
				Strict: true,
				Schema: schema,
			},
		},
	})
	if err != nil {
		return "", fmt.Errorf("ai: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("ai: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("ai: request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("ai: decode response: %w", err)
	}
	if result.Error != nil {
		return "", fmt.Errorf("ai: API error: %s", result.Error.Message)
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("ai: no choices in response")
	}
	return result.Choices[0].Message.Content, nil
}
