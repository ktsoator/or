package httpapi

import (
	"context"
	"encoding/json"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/ktsoator/or/coding/internal/tools"
)

// QuestionBroker asks the browser to answer one set of agent questions and
// waits until it responds or the active run is cancelled. It is the question
// counterpart to ApprovalBroker: same pending-and-resolve shape, but the reply
// carries the user's choices rather than a permission verdict.
//
// There is deliberately no timeout. A question can sit unanswered for as long
// as the user needs to think; abandoning it is what aborting the run is for.
type QuestionBroker struct {
	hub    *Hub
	nextID atomic.Uint64

	mu      sync.Mutex
	pending map[string]pendingQuestion
}

type pendingQuestion struct {
	response  chan []tools.Answer
	questions []tools.Question
}

func NewQuestionBroker(hub *Hub) *QuestionBroker {
	return &QuestionBroker{hub: hub, pending: make(map[string]pendingQuestion)}
}

// Ask implements tools.Asker.
func (b *QuestionBroker) Ask(
	ctx context.Context,
	questions []tools.Question,
) ([]tools.Answer, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	id := strconv.FormatUint(b.nextID.Add(1), 10)
	ch := make(chan []tools.Answer, 1)

	b.mu.Lock()
	b.pending[id] = pendingQuestion{response: ch, questions: questions}
	b.mu.Unlock()

	b.broadcast(wireEvent{
		Type:      wireEventQuestionRequest,
		ID:        id,
		Questions: projectQuestions(questions),
	})

	select {
	case answers := <-ch:
		return answers, nil
	case <-ctx.Done():
		if b.cancel(id) {
			return nil, ctx.Err()
		}
		// Resolve won the race and buffered its answer while cancellation fired.
		return <-ch, nil
	}
}

// PendingEvents returns the questions a refreshed browser must restore.
func (b *QuestionBroker) PendingEvents() []wireEvent {
	b.mu.Lock()
	defer b.mu.Unlock()

	events := make([]wireEvent, 0, len(b.pending))
	for id, pending := range b.pending {
		events = append(events, wireEvent{
			Type:      wireEventQuestionRequest,
			ID:        id,
			Questions: projectQuestions(pending.questions),
		})
	}
	return events
}

func (b *QuestionBroker) HasPending() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.pending) > 0
}

// Resolve atomically claims and answers a pending question.
func (b *QuestionBroker) Resolve(id string, answers []tools.Answer) bool {
	b.mu.Lock()
	pending, ok := b.pending[id]
	if ok {
		delete(b.pending, id)
		pending.response <- answers
	}
	b.mu.Unlock()
	if ok {
		b.broadcast(wireEvent{Type: wireEventQuestionResolved, ID: id})
	}
	return ok
}

func (b *QuestionBroker) cancel(id string) bool {
	b.mu.Lock()
	_, ok := b.pending[id]
	if ok {
		delete(b.pending, id)
	}
	b.mu.Unlock()
	if ok {
		b.broadcast(wireEvent{Type: wireEventQuestionCancelled, ID: id})
	}
	return ok
}

func (b *QuestionBroker) broadcast(event wireEvent) {
	payload, _ := json.Marshal(event)
	b.hub.Broadcast(payload)
}

func projectQuestions(questions []tools.Question) []wireQuestion {
	out := make([]wireQuestion, 0, len(questions))
	for _, question := range questions {
		options := make([]wireQuestionOption, 0, len(question.Options))
		for _, option := range question.Options {
			options = append(options, wireQuestionOption{
				Label:       option.Label,
				Description: option.Description,
			})
		}
		out = append(out, wireQuestion{
			Question:    question.Question,
			Header:      question.Header,
			Options:     options,
			MultiSelect: question.MultiSelect,
		})
	}
	return out
}
