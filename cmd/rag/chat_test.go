package main

import (
	"context"
	"strings"
	"testing"

	"go-rag/internal/core/rag"

	"github.com/google/uuid"
)

type recordingChatRunner struct {
	sessions []string
	messages []string
}

func (r *recordingChatRunner) Run(_ context.Context, sessionID, message string) (rag.State, error) {
	r.sessions = append(r.sessions, sessionID)
	r.messages = append(r.messages, message)
	return rag.State{Answer: "Jawaban " + message}, nil
}

func TestChatLoopUsesOneSessionForEveryTurn(t *testing.T) {
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	runner := &recordingChatRunner{}
	var output strings.Builder
	err = chatLoop(context.Background(), strings.NewReader("Pertanyaan pertama\n\nPertanyaan kedua\n/exit\n"),
		&output, id.String(), runner)
	if err != nil {
		t.Fatal(err)
	}
	if len(runner.sessions) != 2 || runner.sessions[0] != id.String() || runner.sessions[1] != id.String() {
		t.Fatalf("session IDs = %v", runner.sessions)
	}
	if len(runner.messages) != 2 || runner.messages[0] != "Pertanyaan pertama" || runner.messages[1] != "Pertanyaan kedua" {
		t.Fatalf("messages = %v", runner.messages)
	}
	if !strings.Contains(output.String(), "session_id: "+id.String()) ||
		!strings.Contains(output.String(), "AI> Jawaban Pertanyaan kedua") {
		t.Fatalf("output = %q", output.String())
	}
}

func TestChatSessionIDCreatesV7AndValidatesResume(t *testing.T) {
	id, err := chatSessionID("")
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := uuid.Parse(id)
	if err != nil || parsed.Version() != 7 {
		t.Fatalf("generated session ID = %q, err = %v", id, err)
	}
	resumed, err := chatSessionID(id)
	if err != nil || resumed != id {
		t.Fatalf("resumed session ID = %q, err = %v", resumed, err)
	}
	if _, err := chatSessionID(uuid.NewString()); err == nil {
		t.Fatal("UUIDv4 must not be accepted for session resume")
	}
}
