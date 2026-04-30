package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"swarminator/internal/tutorial"
	"swarminator/pkg/llm"
)

type mockProvider struct {
	result string
	err    error
	// captured fields for assertion
	capturedModel   string
	capturedPersona string
	capturedInput   string
}

func (m *mockProvider) Complete(_ context.Context, req llm.CompletionRequest) (string, error) {
	m.capturedModel = req.Model
	m.capturedPersona = req.Persona
	m.capturedInput = req.Input
	return m.result, m.err
}

func TestAnswerTutorialUsesKiloAutoFree(t *testing.T) {
	mock := &mockProvider{result: "some answer from kilo"}
	result := answerTutorialWith(context.Background(), "quickstart", "", mock)
	if mock.capturedModel != tutorialModel {
		t.Errorf("expected model %q, got %q", tutorialModel, mock.capturedModel)
	}
	if result != "some answer from kilo" {
		t.Errorf("unexpected result: %q", result)
	}
}

func TestAnswerTutorialIncludesDocsInInput(t *testing.T) {
	mock := &mockProvider{result: "answer"}
	answerTutorialWith(context.Background(), "myquestion", "", mock)
	if !strings.Contains(mock.capturedInput, "# Swarminator CLI Reference") {
		t.Error("input to provider does not contain embedded docs header")
	}
	if !strings.Contains(mock.capturedInput, "myquestion") {
		t.Error("input to provider does not contain user query")
	}
}

func TestAnswerTutorialFallsBackOnError(t *testing.T) {
	mock := &mockProvider{err: errors.New("kilo unavailable")}
	result := answerTutorialWith(context.Background(), "quickstart", "", mock)
	expected := tutorial.Lookup("quickstart")
	if result != expected {
		t.Errorf("expected fallback result %q, got %q", expected, result)
	}
}

func TestAnswerTutorialFallsBackOnEmptyResponse(t *testing.T) {
	mock := &mockProvider{result: "   "}
	result := answerTutorialWith(context.Background(), "rules", "", mock)
	expected := tutorial.Lookup("rules")
	if result != expected {
		t.Errorf("expected fallback result %q, got %q", expected, result)
	}
}

func TestAnswerTutorialPersonaSet(t *testing.T) {
	mock := &mockProvider{result: "ok"}
	answerTutorialWith(context.Background(), "test", "", mock)
	if mock.capturedPersona != tutorialPersona {
		t.Errorf("persona mismatch: got %q", mock.capturedPersona)
	}
}

func TestAnswerTutorialBlankModelUsesDefault(t *testing.T) {
	mock := &mockProvider{result: "ok"}
	answerTutorialWith(context.Background(), "test", "", mock)
	if mock.capturedModel != tutorialModel {
		t.Errorf("blank model should use default %q, got %q", tutorialModel, mock.capturedModel)
	}
}

func TestAnswerTutorialCustomModelPassedThrough(t *testing.T) {
	mock := &mockProvider{result: "ok"}
	answerTutorialWith(context.Background(), "test", "kilo/custom/model", mock)
	if mock.capturedModel != "kilo/custom/model" {
		t.Errorf("expected custom model %q, got %q", "kilo/custom/model", mock.capturedModel)
	}
}
