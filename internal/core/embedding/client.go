package embedding

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

// Embedder allows the engine to use the configured provider or a test double.
type Embedder interface {
	EmbedPassage(ctx context.Context, text string) ([]float32, error)
}

type Client struct {
	url      string
	apiKey   string
	model    string
	provider string
	http     *http.Client
}

func NewClient(cfg configs.EmbeddingRAGConfig, httpClient *http.Client) (*Client, error) {
	if cfg.BaseURL == "" || cfg.APIKey == "" || cfg.Model == "" {
		return nil, fmt.Errorf("embedding base URL, API key, and model are required")
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{
		url:      strings.TrimRight(cfg.BaseURL, "/") + "/embeddings",
		apiKey:   cfg.APIKey,
		model:    cfg.Model,
		provider: cfg.Provider,
		http:     httpClient,
	}, nil
}

func (c *Client) EmbedPassage(ctx context.Context, text string) ([]float32, error) {
	return c.embed(ctx, text, "passage")
}

func (c *Client) EmbedQuery(ctx context.Context, text string) ([]float32, error) {
	return c.embed(ctx, text, "query")
}

func (c *Client) embed(ctx context.Context, text, inputType string) ([]float32, error) {
	payload, err := json.Marshal(struct {
		Model          string `json:"model"`
		Input          string `json:"input"`
		InputType      string `json:"input_type,omitempty"`
		EncodingFormat string `json:"encoding_format"`
	}{c.model, text, providerInputType(c.provider, inputType), "float"})
	if err != nil {
		return nil, fmt.Errorf("encode embedding request: %w", err)
	}
	for attempt := 0; attempt < 3; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(payload))
		if err != nil {
			return nil, fmt.Errorf("create embedding request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
		req.Header.Set("Content-Type", "application/json")

		resp, err := c.http.Do(req)
		if err != nil {
			if attempt == 2 || ctx.Err() != nil {
				return nil, fmt.Errorf("embedding request: %w", err)
			}
		} else if resp.StatusCode == http.StatusOK {
			var result struct {
				Data []struct {
					Embedding []float32 `json:"embedding"`
				} `json:"data"`
			}
			decodeErr := json.NewDecoder(resp.Body).Decode(&result)
			resp.Body.Close()
			if decodeErr != nil {
				return nil, fmt.Errorf("decode embedding response: %w", decodeErr)
			}
			if len(result.Data) != 1 || len(result.Data[0].Embedding) == 0 {
				return nil, fmt.Errorf("embedding provider returned no vector")
			}
			return result.Data[0].Embedding, nil
		} else {
			resp.Body.Close()
			if attempt == 2 || (resp.StatusCode != http.StatusTooManyRequests && resp.StatusCode < 500) {
				return nil, fmt.Errorf("embedding provider returned HTTP %d", resp.StatusCode)
			}
		}
		timer := time.NewTimer(time.Duration(1<<attempt) * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
	return nil, fmt.Errorf("embedding request failed after retries")
}

func providerInputType(provider, inputType string) string {
	if provider == "openai" {
		return ""
	}
	return inputType
}
