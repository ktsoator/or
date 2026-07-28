package usage

import (
	"testing"

	"github.com/ktsoator/or/llm"
)

func TestAddTotalsPreservesUnknownInput(t *testing.T) {
	var total Totals
	addTotals(&total, llm.Usage{InputUnknown: true, Output: 5, TotalTokens: 5})
	addTotals(&total, llm.Usage{Input: 3, Output: 2, TotalTokens: 5})

	if total.Requests != 2 || !total.InputUnknown || total.Input != 3 || total.Output != 7 || total.TotalTokens != 10 {
		t.Fatalf("totals = %#v, want known input retained and aggregate marked unknown", total)
	}
}
