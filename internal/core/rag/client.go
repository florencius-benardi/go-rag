package rag

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go-rag/internal/configs"
)

// chatAttempts is the total number of tries per completion. The provider drops
// requests often enough that a single retry left outages visible to guests.
const chatAttempts = 3

type ChatClient struct {
	url, apiKey, model string
	http               *http.Client
	retryBase          time.Duration
	onRetry            func(attempt int, wait time.Duration, err error)
}

func NewChatClient(cfg configs.LLMConfig, client *http.Client) (*ChatClient, error) {
	if cfg.BaseURL == "" || cfg.APIKey == "" || cfg.Model == "" {
		return nil, fmt.Errorf("LLM_BASE_URL, LLM_API_KEY, and LLM_MODEL are required")
	}
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	return &ChatClient{url: strings.TrimRight(cfg.BaseURL, "/") + "/chat/completions",
		apiKey: cfg.APIKey, model: cfg.Model, http: client, retryBase: time.Second}, nil
}

// WithRetryLogger registers a callback invoked only when a failed attempt is
// about to be retried. It never fires on a successful call, and never on the
// final failure, which the caller already sees as an error.
func (c *ChatClient) WithRetryLogger(fn func(attempt int, wait time.Duration, err error)) *ChatClient {
	c.onRetry = fn
	return c
}

func (c *ChatClient) Complete(ctx context.Context, system, user string) (string, error) {
	request := struct {
		Model       string    `json:"model"`
		Messages    []Message `json:"messages"`
		Temperature float64   `json:"temperature"`
		Stream      bool      `json:"stream"`
	}{c.model, []Message{{Role: "system", Content: system}, {Role: "user", Content: user}}, 0, false}
	body, err := json.Marshal(request)
	if err != nil {
		return "", err
	}
	var lastErr error
	for attempt := 1; attempt <= chatAttempts; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(body))
		if err != nil {
			return "", err
		}
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
		req.Header.Set("Content-Type", "application/json")
		response, err := c.http.Do(req)
		if err != nil {
			lastErr = err
		} else {
			var result struct {
				Choices []struct {
					Message Message `json:"message"`
				} `json:"choices"`
			}
			decodeErr := json.NewDecoder(response.Body).Decode(&result)
			response.Body.Close()
			if response.StatusCode == http.StatusOK && decodeErr == nil && len(result.Choices) > 0 {
				return strings.TrimSpace(result.Choices[0].Message.Content), nil
			}
			lastErr = fmt.Errorf("chat provider HTTP %d", response.StatusCode)
			// A 4xx other than rate limiting is our fault; retrying repeats it.
			if response.StatusCode != http.StatusTooManyRequests && response.StatusCode < 500 {
				break
			}
		}
		if attempt == chatAttempts {
			break
		}
		wait := c.retryBase << (attempt - 1)
		if c.onRetry != nil {
			c.onRetry(attempt, wait, lastErr)
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(wait):
		}
	}
	return "", lastErr
}

func (c *ChatClient) Model() string { return c.model }
