package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/llm"
)

type stubAsker struct {
	questions []Question
	answers   []Answer
	err       error
}

func (s *stubAsker) Ask(_ context.Context, questions []Question) ([]Answer, error) {
	s.questions = questions
	return s.answers, s.err
}

func askArgs(t *testing.T, questions ...Question) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(askUserQuestionArgs{Questions: questions})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func twoOptions() []Option {
	return []Option{
		{Label: "Redis", Description: "shared across instances"},
		{Label: "In-memory", Description: "no dependency"},
	}
}

func execute(t *testing.T, tool Tool, raw json.RawMessage) (agent.ToolResult, error) {
	t.Helper()
	return tool.Execute(context.Background(), "call-1", raw, nil)
}

func resultText(t *testing.T, result agent.ToolResult) string {
	t.Helper()
	var text string
	for _, content := range result.Content {
		if block, ok := content.(*llm.TextContent); ok {
			text += block.Text
		}
	}
	return text
}

// The counts belong in the schema so the agent rejects a malformed call before
// the tool runs and the model gets a precise validation error.
func TestQuestionSchemaBoundsTheCounts(t *testing.T) {
	schema := string(AskUserQuestion(&stubAsker{}).Definition.Parameters)
	for _, want := range []string{
		`"minItems":1`,
		`"maxItems":4`,
		`"minItems":2`,
	} {
		if !strings.Contains(schema, want) {
			t.Errorf("schema missing %q:\n%s", want, schema)
		}
	}
}

func TestQuestionRejectsDuplicatesAndOversizedHeaders(t *testing.T) {
	tool := AskUserQuestion(&stubAsker{})
	tests := map[string]struct {
		questions []Question
		want      string
	}{
		"duplicate question": {
			questions: []Question{
				{Question: "Which cache?", Header: "Cache", Options: twoOptions()},
				{Question: "Which cache?", Header: "Cache 2", Options: twoOptions()},
			},
			want: "asked twice",
		},
		"duplicate option": {
			questions: []Question{{
				Question: "Which cache?",
				Header:   "Cache",
				Options: []Option{
					{Label: "Redis", Description: "one"},
					{Label: "Redis", Description: "two"},
				},
			}},
			want: "offered twice",
		},
		"missing header": {
			questions: []Question{
				{Question: "Which cache?", Header: "  ", Options: twoOptions()},
			},
			want: "needs a short header",
		},
		"oversized header": {
			questions: []Question{
				{Question: "Which cache?", Header: "a header far too long", Options: twoOptions()},
			},
			want: "longer than 12 characters",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			result, err := execute(t, tool, askArgs(t, test.questions...))
			if err == nil {
				t.Fatal("invalid questions were accepted")
			}
			// The model reads the result text, so the correction must be there
			// too, not only in the returned error.
			if !strings.Contains(resultText(t, result), test.want) {
				t.Errorf("result = %q, want it to mention %q", resultText(t, result), test.want)
			}
		})
	}
}

func TestQuestionReturnsAnswersToTheModelAndTheProductShell(t *testing.T) {
	asker := &stubAsker{answers: []Answer{
		{Question: "Which cache?", Values: []string{"Redis"}},
		{Question: "Which features?", Values: []string{"Retries", "Metrics"}},
	}}
	tool := AskUserQuestion(asker)
	result, err := execute(t, tool, askArgs(t,
		Question{Question: "Which cache?", Header: "Cache", Options: twoOptions()},
		Question{Question: "Which features?", Header: "Features", Options: twoOptions(), MultiSelect: true},
	))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, result)
	for _, want := range []string{
		`<answer question="Which cache?">Redis</answer>`,
		`<answer question="Which features?">Retries, Metrics</answer>`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("model-facing text missing %q:\n%s", want, text)
		}
	}

	details, ok := result.Outcome.Data.(QuestionAnswers)
	if !ok {
		t.Fatalf("outcome data = %T, want QuestionAnswers", result.Outcome.Data)
	}
	if result.Outcome.Status != agent.ToolOutcomeSuccess {
		t.Fatalf("outcome status = %q, want success", result.Outcome.Status)
	}
	if len(details.Questions) != 2 || len(details.Answers) != 2 {
		t.Fatalf("details = %#v, want both questions and both answers", details)
	}
	if len(asker.questions) != 2 || asker.questions[0].Options[0].Label != "Redis" {
		t.Fatalf("asker received %#v", asker.questions)
	}
}

// Silence must never read as agreement: a question the surface returns no
// answer for is reported as unanswered rather than filled in.
func TestQuestionReportsAnUnansweredQuestionRatherThanInventingOne(t *testing.T) {
	asker := &stubAsker{answers: []Answer{
		{Question: "Which cache?", Values: []string{"  "}},
	}}
	tool := AskUserQuestion(asker)
	result, err := execute(t, tool, askArgs(t,
		Question{Question: "Which cache?", Header: "Cache", Options: twoOptions()},
	))
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(t, result)
	if !strings.Contains(text, `<unanswered question="Which cache?" />`) {
		t.Fatalf("blank answer was not reported as unanswered:\n%s", text)
	}
}

// An answer to something that was not asked is dropped, and the remaining
// questions keep the order the model asked them in.
func TestQuestionAlignsAnswersToTheQuestionsAsked(t *testing.T) {
	asker := &stubAsker{answers: []Answer{
		{Question: "Never asked", Values: []string{"ignored"}},
		{Question: "Second", Values: []string{"b"}},
		{Question: "First", Values: []string{"a"}},
	}}
	tool := AskUserQuestion(asker)
	result, err := execute(t, tool, askArgs(t,
		Question{Question: "First", Header: "One", Options: twoOptions()},
		Question{Question: "Second", Header: "Two", Options: twoOptions()},
	))
	if err != nil {
		t.Fatal(err)
	}
	details := result.Outcome.Data.(QuestionAnswers)
	if len(details.Answers) != 2 ||
		details.Answers[0].Question != "First" ||
		details.Answers[1].Question != "Second" {
		t.Fatalf("answers = %#v, want them aligned to the questions asked", details.Answers)
	}
	if strings.Contains(resultText(t, result), "ignored") {
		t.Error("an answer to an unasked question reached the model")
	}
}

func TestQuestionSurfacesAskerFailureWithoutFabricatingAnswers(t *testing.T) {
	cancelled := errors.New("run aborted")
	tool := AskUserQuestion(&stubAsker{err: cancelled})
	result, err := execute(t, tool, askArgs(t,
		Question{Question: "Which cache?", Header: "Cache", Options: twoOptions()},
	))
	if !errors.Is(err, cancelled) {
		t.Fatalf("error = %v, want the asker's failure", err)
	}
	if len(result.Content) != 0 || result.Outcome.Data != nil {
		t.Fatalf("a failed question produced a result: %#v", result)
	}
}
