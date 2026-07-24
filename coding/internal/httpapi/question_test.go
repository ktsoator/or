package httpapi

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ktsoator/or/coding/internal/tools"
)

type questionResult struct {
	answers []tools.Answer
	err     error
}

func cacheQuestions() []tools.Question {
	return []tools.Question{{
		Question: "Which cache?",
		Header:   "Cache",
		Options: []tools.Option{
			{Label: "Redis", Description: "shared across instances"},
			{Label: "In-memory", Description: "no dependency"},
		},
	}}
}

func TestQuestionBrokerDeliversAnswers(t *testing.T) {
	hub := NewHub()
	events, syncRequired := hub.add(0)
	if syncRequired {
		t.Fatal("unexpected sync requirement")
	}
	defer hub.remove(events)
	broker := NewQuestionBroker(hub)
	result := make(chan questionResult, 1)

	go func() {
		answers, err := broker.Ask(context.Background(), cacheQuestions())
		result <- questionResult{answers: answers, err: err}
	}()

	requested := readApprovalEvent(t, events)
	if requested.Type != "question_request" || requested.ID == "" {
		t.Fatalf("request event = %+v", requested)
	}
	// The browser must receive everything it needs to render the choice.
	if len(requested.Questions) != 1 ||
		requested.Questions[0].Header != "Cache" ||
		len(requested.Questions[0].Options) != 2 ||
		requested.Questions[0].Options[0].Label != "Redis" {
		t.Fatalf("projected questions = %+v", requested.Questions)
	}

	if !broker.Resolve(requested.ID, []tools.Answer{
		{Question: "Which cache?", Values: []string{"Redis"}},
	}) {
		t.Fatal("Resolve returned false")
	}
	resolved := readApprovalEvent(t, events)
	if resolved.Type != "question_resolved" || resolved.ID != requested.ID {
		t.Fatalf("resolved event = %+v", resolved)
	}

	select {
	case got := <-result:
		if got.err != nil ||
			len(got.answers) != 1 ||
			got.answers[0].Values[0] != "Redis" {
			t.Fatalf("Ask() = %+v, %v", got.answers, got.err)
		}
	case <-time.After(time.Second):
		t.Fatal("Ask did not return")
	}
	if broker.HasPending() {
		t.Fatal("broker still has a pending question")
	}
}

func TestQuestionBrokerCancelsWithoutAnswering(t *testing.T) {
	hub := NewHub()
	events, syncRequired := hub.add(0)
	if syncRequired {
		t.Fatal("unexpected sync requirement")
	}
	defer hub.remove(events)
	broker := NewQuestionBroker(hub)
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan questionResult, 1)

	go func() {
		answers, err := broker.Ask(ctx, cacheQuestions())
		result <- questionResult{answers: answers, err: err}
	}()

	requested := readApprovalEvent(t, events)
	cancel()
	cancelled := readApprovalEvent(t, events)
	if cancelled.Type != "question_cancelled" || cancelled.ID != requested.ID {
		t.Fatalf("cancelled event = %+v", cancelled)
	}
	select {
	case got := <-result:
		if !errors.Is(got.err, context.Canceled) {
			t.Fatalf("Ask error = %v, want context.Canceled", got.err)
		}
		// An aborted question must not look like an answered one.
		if got.answers != nil {
			t.Fatalf("cancelled Ask returned answers: %+v", got.answers)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled Ask did not return")
	}
	if broker.HasPending() {
		t.Fatal("broker still has a pending question")
	}
}

// Resolve and cancellation can fire together. When Resolve claimed the question
// first its answer must still reach the caller rather than being dropped.
func TestQuestionBrokerKeepsAnAnswerThatWonTheCancellationRace(t *testing.T) {
	hub := NewHub()
	events, syncRequired := hub.add(0)
	if syncRequired {
		t.Fatal("unexpected sync requirement")
	}
	defer hub.remove(events)
	broker := NewQuestionBroker(hub)
	ctx, cancel := context.WithCancel(context.Background())

	started := make(chan string, 1)
	result := make(chan questionResult, 1)
	go func() {
		id := readApprovalEvent(t, events).ID
		started <- id
	}()
	go func() {
		answers, err := broker.Ask(ctx, cacheQuestions())
		result <- questionResult{answers: answers, err: err}
	}()

	id := <-started
	// Claim the question before cancelling, mirroring an answer that lands as
	// the run is being aborted.
	if !broker.Resolve(id, []tools.Answer{{Question: "Which cache?", Values: []string{"Redis"}}}) {
		t.Fatal("Resolve returned false")
	}
	cancel()

	select {
	case got := <-result:
		if got.err != nil || len(got.answers) != 1 || got.answers[0].Values[0] != "Redis" {
			t.Fatalf("Ask() = %+v, %v, want the resolved answer", got.answers, got.err)
		}
	case <-time.After(time.Second):
		t.Fatal("Ask did not return after winning the race")
	}
}

func TestQuestionBrokerRestoresPendingQuestionsForARefreshedBrowser(t *testing.T) {
	hub := NewHub()
	events, _ := hub.add(0)
	defer hub.remove(events)
	broker := NewQuestionBroker(hub)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() { _, _ = broker.Ask(ctx, cacheQuestions()) }()
	requested := readApprovalEvent(t, events)

	pending := broker.PendingEvents()
	if len(pending) != 1 ||
		pending[0].ID != requested.ID ||
		pending[0].Type != "question_request" ||
		len(pending[0].Questions) != 1 {
		t.Fatalf("pending events = %+v", pending)
	}
}

func TestQuestionBrokerRejectsAnUnknownID(t *testing.T) {
	broker := NewQuestionBroker(NewHub())
	if broker.Resolve("missing", []tools.Answer{{Question: "q", Values: []string{"a"}}}) {
		t.Fatal("Resolve accepted an unknown question id")
	}
}
