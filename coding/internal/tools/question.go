package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/llm"
)

// ToolNameAskUserQuestion is the advertised name of the question tool.
const ToolNameAskUserQuestion = "ask_user_question"

// Question limits. The schema enforces the counts, so a model that exceeds them
// gets a validation error before the tool runs; Execute only checks what a
// schema cannot express.
const (
	MinQuestions   = 1
	MaxQuestions   = 4
	MinOptions     = 2
	MaxOptions     = 4
	MaxHeaderRunes = 12
)

// Option is one selectable answer to a Question.
type Option struct {
	Label string `json:"label" jsonschema:"description=Display text for this option. Keep it to a few words,minLength=1"`
	// Description explains what choosing this option means. It gives the user
	// the trade-off the model already knows but the label has no room for.
	Description string `json:"description" jsonschema:"description=What this option means or what happens if it is chosen. State the trade-off"`
}

// Question is one multiple-choice question put to the user. The same type is
// both the model-facing schema and the value handed to an Asker so the product
// shell renders exactly what the model asked.
type Question struct {
	Question string `json:"question" jsonschema:"description=The complete question. Make it specific and end it with a question mark,minLength=1"`
	// Header labels the question in the product surface, where a full sentence
	// does not fit.
	Header      string   `json:"header" jsonschema:"description=Very short label shown as a chip beside the question such as Auth method or Library,minLength=1"`
	Options     []Option `json:"options" jsonschema:"description=The available choices. Each must be distinct,minItems=2,maxItems=4"`
	MultiSelect bool     `json:"multi_select,omitempty" jsonschema:"description=Allow several options to be selected at once. Set it when the options are not mutually exclusive"`
}

// Answer is the user's response to one Question. Values holds one entry for a
// single-select question and may hold several for a multi-select one; free text
// the user typed instead of picking an option arrives here unchanged.
type Answer struct {
	Question string   `json:"question"`
	Values   []string `json:"values"`
}

// QuestionAnswers is the structured result product shells render.
type QuestionAnswers struct {
	Questions []Question `json:"questions"`
	Answers   []Answer   `json:"answers"`
}

// Asker puts questions to the user and blocks until they answer. Implementations
// must honor ctx cancellation, so aborting a run cannot leave a question
// waiting forever, and must not invent an answer the user did not give: a
// cancelled or abandoned question is an error, not an empty answer.
type Asker interface {
	Ask(context.Context, []Question) ([]Answer, error)
}

// ErrNoAsker is returned when no product surface can reach the user.
var ErrNoAsker = errors.New("no interactive surface is available to ask the user")

type askUserQuestionArgs struct {
	Questions []Question `json:"questions" jsonschema:"description=The questions to ask. Batch related questions into one call rather than asking them one at a time,minItems=1,maxItems=4"`
}

// AskUserQuestion returns the tool that puts a decision back to the user. It is
// only registered when a product surface can actually reach them; a session with
// nobody at the keyboard advertises no question tool rather than one that always
// fails.
func AskUserQuestion(asker Asker) Tool {
	def := llm.MustTool[askUserQuestionArgs](ToolNameAskUserQuestion, questionText.description)
	return Tool{
		AgentTool: agent.AgentTool{
			Definition: def,
			Label:      "Ask",
			Execute: func(
				ctx context.Context,
				_ string,
				raw json.RawMessage,
				_ func(agent.ToolProgress),
			) (agent.ToolResult, error) {
				var in askUserQuestionArgs
				if err := json.Unmarshal(raw, &in); err != nil {
					return agent.ToolResult{}, err
				}
				if err := validateQuestions(in.Questions); err != nil {
					return textResult(err.Error()), err
				}
				if asker == nil {
					return textResult(ErrNoAsker.Error()), ErrNoAsker
				}
				answers, err := asker.Ask(ctx, in.Questions)
				if err != nil {
					return agent.ToolResult{}, err
				}
				answers = alignAnswers(in.Questions, answers)
				return resultWith(
					renderAnswers(answers),
					QuestionAnswers{Questions: in.Questions, Answers: answers},
				), nil
			},
		},
		AccessFor:  InternalAccess,
		Guidelines: questionText.guidelines,
	}
}

// validateQuestions checks the constraints the JSON schema cannot express. The
// messages name the fix so the model corrects the call instead of retrying it
// unchanged.
func validateQuestions(questions []Question) error {
	seenQuestions := make(map[string]bool, len(questions))
	for _, question := range questions {
		text := strings.TrimSpace(question.Question)
		if text == "" {
			return errors.New("ask_user_question: every question needs non-empty question text")
		}
		if seenQuestions[text] {
			return fmt.Errorf(
				"ask_user_question: question %q is asked twice; every question must be distinct",
				text,
			)
		}
		seenQuestions[text] = true

		if header := strings.TrimSpace(question.Header); header == "" {
			return fmt.Errorf("ask_user_question: question %q needs a short header", text)
		} else if len([]rune(header)) > MaxHeaderRunes {
			return fmt.Errorf(
				"ask_user_question: header %q for question %q is longer than %d characters",
				header,
				text,
				MaxHeaderRunes,
			)
		}

		seenLabels := make(map[string]bool, len(question.Options))
		for _, option := range question.Options {
			label := strings.TrimSpace(option.Label)
			if label == "" {
				return fmt.Errorf(
					"ask_user_question: every option of question %q needs a non-empty label",
					text,
				)
			}
			if seenLabels[label] {
				return fmt.Errorf(
					"ask_user_question: option %q of question %q is offered twice; every option must be distinct",
					label,
					text,
				)
			}
			seenLabels[label] = true
		}
	}
	return nil
}

// alignAnswers puts answers in the order the questions were asked and drops any
// answer to a question that was not asked. A question the surface left out
// arrives with no values rather than a fabricated one, so renderAnswers can
// report it as unanswered.
func alignAnswers(questions []Question, answers []Answer) []Answer {
	byQuestion := make(map[string][]string, len(answers))
	for _, answer := range answers {
		byQuestion[strings.TrimSpace(answer.Question)] = usableValues(answer.Values)
	}
	aligned := make([]Answer, len(questions))
	for index, question := range questions {
		text := strings.TrimSpace(question.Question)
		aligned[index] = Answer{Question: text, Values: byQuestion[text]}
	}
	return aligned
}

func usableValues(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// renderAnswers is what the model reads. An unanswered question says so
// explicitly: silence must not read as consent to whatever the model proposed.
func renderAnswers(answers []Answer) string {
	var b strings.Builder
	b.WriteString("<user-answers>\n")
	for _, answer := range answers {
		if len(answer.Values) == 0 {
			fmt.Fprintf(
				&b,
				"<unanswered question=%q />\n",
				answer.Question,
			)
			continue
		}
		fmt.Fprintf(
			&b,
			"<answer question=%q>%s</answer>\n",
			answer.Question,
			strings.Join(answer.Values, ", "),
		)
	}
	b.WriteString("</user-answers>")
	return b.String()
}
