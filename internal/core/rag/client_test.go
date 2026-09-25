package rag

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go-rag/internal/configs"
)

// newTestClient points a client at srv with a negligible backoff so the retry
// path can be exercised without real waiting.
func newTestClient(t *testing.T, srv *httptest.Server) *ChatClient {
	t.Helper()
	client, err := NewChatClient(configs.LLMConfig{BaseURL: srv.URL, APIKey: "k", Model: "m"}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	client.retryBase = time.Millisecond
	return client
}

func TestCompleteRetriesServerErrorAndLogsOnlyTheRetry(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		if calls < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"halo"}}]}`))
	}))
	defer srv.Close()

	var retries []int
	client := newTestClient(t, srv).WithRetryLogger(func(attempt int, wait time.Duration, err error) {
		if wait <= 0 || err == nil {
			t.Errorf("retry logged without a wait or cause: wait=%s err=%v", wait, err)
		}
		retries = append(retries, attempt)
	})
	answer, err := client.Complete(context.Background(), "system", "user")
	if err != nil || answer != "halo" {
		t.Fatalf("transient failure was not recovered: answer=%q err=%v", answer, err)
	}
	// Two failures, two retries, three calls. The success is never logged.
	if calls != 3 || len(retries) != 2 || retries[0] != 1 || retries[1] != 2 {
		t.Fatalf("unexpected retry pattern: calls=%d retries=%v", calls, retries)
	}
}

func TestCompleteDoesNotRetryOrLogClientError(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	logged := 0
	client := newTestClient(t, srv).WithRetryLogger(func(int, time.Duration, error) { logged++ })
	if _, err := client.Complete(context.Background(), "system", "user"); err == nil {
		t.Fatal("client error was reported as success")
	}
	if calls != 1 || logged != 0 {
		t.Fatalf("client error was retried: calls=%d logged=%d", calls, logged)
	}
}

func TestCompleteLogsNothingWhenFirstAttemptSucceeds(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"halo"}}]}`))
	}))
	defer srv.Close()

	logged := 0
	client := newTestClient(t, srv).WithRetryLogger(func(int, time.Duration, error) { logged++ })
	if _, err := client.Complete(context.Background(), "system", "user"); err != nil {
		t.Fatal(err)
	}
	if logged != 0 {
		t.Fatalf("successful call was logged as a retry: logged=%d", logged)
	}
}
