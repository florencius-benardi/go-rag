package embedding

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"go-rag/internal/configs"
)

func TestClientUsesPassageMode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings" || r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("unexpected request path or authorization")
		}
		var body struct {
			Model     string `json:"model"`
			Input     string `json:"input"`
			InputType string `json:"input_type"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if body.Model != "test-model" || body.Input != "Kategori: Dessert" || body.InputType != "passage" {
			t.Errorf("unexpected embedding payload: %+v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"embedding":[0.1,0.2]}]}`))
	}))
	defer server.Close()

	client, err := NewClient(configs.EmbeddingRAGConfig{
		BaseURL: server.URL + "/v1", APIKey: "test-key", Model: "test-model",
	}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	vector, err := client.EmbedPassage(context.Background(), "Kategori: Dessert")
	if err != nil {
		t.Fatal(err)
	}
	if len(vector) != 2 {
		t.Fatalf("vector length = %d", len(vector))
	}
}

func TestClientOpenAIProviderOmitsInputType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if _, exists := body["input_type"]; exists {
			t.Error("openai provider received input_type")
		}
		_, _ = w.Write([]byte(`{"data":[{"embedding":[0.1,0.2]}]}`))
	}))
	defer server.Close()
	client, err := NewClient(configs.EmbeddingRAGConfig{Provider: "openai", BaseURL: server.URL,
		APIKey: "test", Model: "test"}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.EmbedQuery(context.Background(), "hello"); err != nil {
		t.Fatal(err)
	}
}

func TestClientUsesQueryModeForRetrieval(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			InputType string `json:"input_type"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body.InputType != "query" {
			t.Errorf("input_type = %q", body.InputType)
		}
		_, _ = w.Write([]byte(`{"data":[{"embedding":[0.1,0.2]}]}`))
	}))
	defer server.Close()
	client, err := NewClient(configs.EmbeddingRAGConfig{Provider: "nvidia", BaseURL: server.URL,
		APIKey: "test", Model: "test"}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.EmbedQuery(context.Background(), "dessert"); err != nil {
		t.Fatal(err)
	}
}
