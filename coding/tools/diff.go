package tools

import "strings"

// diffContext is the number of unchanged lines kept around each change when
// grouping the edit script into hunks.
const diffContext = 3

// diffLines computes a line-level diff between old and new and returns it as
// unified-diff hunks with surrounding context, plus the total added and deleted
// line counts. Identical inputs yield no hunks. The algorithm is a longest
// common subsequence over lines, kept dependency-free in the spirit of the rest
// of this package; it is O(n*m) in time and memory, which is fine for the file
// sizes the tools read.
func diffLines(old, new string) (hunks []Hunk, additions, deletions int) {
	a := splitLines(old)
	b := splitLines(new)
	ops := diffOps(a, b)

	for _, op := range ops {
		switch op.kind {
		case opIns:
			additions++
		case opDel:
			deletions++
		}
	}
	if additions == 0 && deletions == 0 {
		return nil, 0, 0
	}
	return buildHunks(ops), additions, deletions
}

// splitLines splits file content into lines, dropping a single trailing newline
// so "a\nb\n" is two lines, not three. An empty string is zero lines.
func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
}

type opKind int

const (
	opEq opKind = iota
	opDel
	opIns
)

// diffOp is one step of the edit script. oldLine and newLine are the 1-based
// line numbers on each side; the side that does not participate is zero.
type diffOp struct {
	kind    opKind
	text    string
	oldLine int
	newLine int
}

// diffOps aligns a and b via an LCS table and emits the edit script in order.
func diffOps(a, b []string) []diffOp {
	m, n := len(a), len(b)
	// dp[i][j] = length of the LCS of a[i:] and b[j:].
	dp := make([][]int, m+1)
	for i := range dp {
		dp[i] = make([]int, n+1)
	}
	for i := m - 1; i >= 0; i-- {
		for j := n - 1; j >= 0; j-- {
			if a[i] == b[j] {
				dp[i][j] = dp[i+1][j+1] + 1
			} else if dp[i+1][j] >= dp[i][j+1] {
				dp[i][j] = dp[i+1][j]
			} else {
				dp[i][j] = dp[i][j+1]
			}
		}
	}

	var ops []diffOp
	i, j := 0, 0
	for i < m && j < n {
		switch {
		case a[i] == b[j]:
			ops = append(ops, diffOp{kind: opEq, text: a[i], oldLine: i + 1, newLine: j + 1})
			i++
			j++
		case dp[i+1][j] >= dp[i][j+1]:
			ops = append(ops, diffOp{kind: opDel, text: a[i], oldLine: i + 1})
			i++
		default:
			ops = append(ops, diffOp{kind: opIns, text: b[j], newLine: j + 1})
			j++
		}
	}
	for ; i < m; i++ {
		ops = append(ops, diffOp{kind: opDel, text: a[i], oldLine: i + 1})
	}
	for ; j < n; j++ {
		ops = append(ops, diffOp{kind: opIns, text: b[j], newLine: j + 1})
	}
	return ops
}

// buildHunks groups the edit script into hunks, each padded with up to
// diffContext unchanged lines and merged when their context windows touch.
func buildHunks(ops []diffOp) []Hunk {
	// Indices of changed ops.
	var changes []int
	for i, op := range ops {
		if op.kind != opEq {
			changes = append(changes, i)
		}
	}
	if len(changes) == 0 {
		return nil
	}

	// Merge change indices into [start, end] op ranges including context.
	type span struct{ start, end int }
	var spans []span
	for _, idx := range changes {
		start := idx - diffContext
		if start < 0 {
			start = 0
		}
		end := idx + diffContext
		if end > len(ops)-1 {
			end = len(ops) - 1
		}
		if n := len(spans); n > 0 && start <= spans[n-1].end+1 {
			if end > spans[n-1].end {
				spans[n-1].end = end
			}
			continue
		}
		spans = append(spans, span{start: start, end: end})
	}

	hunks := make([]Hunk, 0, len(spans))
	for _, s := range spans {
		h := Hunk{}
		for k := s.start; k <= s.end; k++ {
			op := ops[k]
			switch op.kind {
			case opEq:
				setHunkStarts(&h, op.oldLine, op.newLine)
				h.OldLines++
				h.NewLines++
				h.Lines = append(h.Lines, " "+op.text)
			case opDel:
				setHunkStarts(&h, op.oldLine, 0)
				h.OldLines++
				h.Lines = append(h.Lines, "-"+op.text)
			case opIns:
				setHunkStarts(&h, 0, op.newLine)
				h.NewLines++
				h.Lines = append(h.Lines, "+"+op.text)
			}
		}
		hunks = append(hunks, h)
	}
	return hunks
}

// setHunkStarts records the first old/new line number seen in a hunk.
func setHunkStarts(h *Hunk, oldLine, newLine int) {
	if h.OldStart == 0 && oldLine > 0 {
		h.OldStart = oldLine
	}
	if h.NewStart == 0 && newLine > 0 {
		h.NewStart = newLine
	}
}
