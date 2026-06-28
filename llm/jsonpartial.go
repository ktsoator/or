package llm

import "strings"

// completeJSON closes the open containers and string of a truncated JSON
// document, returning a syntactically complete string. It tracks the longest
// prefix that, once its open containers are closed, parses as valid JSON, and
// discards any trailing partial token (an unterminated number, literal, key, or
// dangling separator). An unterminated string in a value position is closed in
// place so its received characters are kept. ok is false when no non-empty
// closeable prefix exists.
//
// It salvages tool arguments whose JSON stream was cut short. Callers must still
// decode the result, which rejects any completion that does not parse.
func completeJSON(raw string) (string, bool) {
	const (
		topVal   = iota // expecting the single top-level value
		topDone         // top-level value consumed
		objKey          // after '{' or ',', expecting a key string or '}'
		objColon        // after a key, expecting ':'
		objVal          // after ':', expecting a value
		objComma        // after a value, expecting ',' or '}'
		arrVal          // after '[' or ',', expecting a value or ']'
		arrComma        // after a value, expecting ',' or ']'
	)

	afterValue := func(state int) int {
		switch state {
		case objVal:
			return objComma
		case arrVal:
			return arrComma
		default:
			return topDone
		}
	}

	var closers []byte // pending '}' / ']' for open containers, outermost first
	var saved []int    // parent state to resume when each container closes
	state := topVal

	safeLen := -1
	var safeClosers []byte
	mark := func(pos int) {
		safeLen = pos
		safeClosers = append(safeClosers[:0], closers...)
	}

	fallback := func() (string, bool) {
		if safeLen < 0 {
			return "", false
		}
		var b strings.Builder
		b.WriteString(raw[:safeLen])
		appendClosers(&b, safeClosers)
		return b.String(), true
	}

	for i := 0; i < len(raw); {
		c := raw[i]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			i++
			continue
		}

		switch state {
		case topVal, objVal, arrVal:
			switch {
			case c == '{':
				saved = append(saved, afterValue(state))
				closers = append(closers, '}')
				state = objKey
				i++
				mark(i)
			case c == '[':
				saved = append(saved, afterValue(state))
				closers = append(closers, ']')
				state = arrVal
				i++
				mark(i)
			case c == '"':
				end, ok := scanString(raw, i)
				if !ok {
					return closeOpenString(raw, i, closers), true
				}
				i = end
				state = afterValue(state)
				mark(i)
			case c == 't' || c == 'f' || c == 'n':
				end, ok := scanLiteral(raw, i)
				if !ok {
					return fallback()
				}
				i = end
				state = afterValue(state)
				mark(i)
			case c == '-' || (c >= '0' && c <= '9'):
				end, ok := scanNumber(raw, i)
				if !ok {
					return fallback()
				}
				i = end
				state = afterValue(state)
				mark(i)
			default:
				return fallback()
			}
		case objKey:
			switch {
			case c == '}':
				closers = closers[:len(closers)-1]
				state = saved[len(saved)-1]
				saved = saved[:len(saved)-1]
				i++
				mark(i)
			case c == '"':
				end, ok := scanString(raw, i)
				if !ok {
					return fallback()
				}
				i = end
				state = objColon
			default:
				return fallback()
			}
		case objColon:
			if c != ':' {
				return fallback()
			}
			state = objVal
			i++
		case objComma:
			switch c {
			case ',':
				state = objKey
				i++
			case '}':
				closers = closers[:len(closers)-1]
				state = saved[len(saved)-1]
				saved = saved[:len(saved)-1]
				i++
				mark(i)
			default:
				return fallback()
			}
		case arrComma:
			switch c {
			case ',':
				state = arrVal
				i++
			case ']':
				closers = closers[:len(closers)-1]
				state = saved[len(saved)-1]
				saved = saved[:len(saved)-1]
				i++
				mark(i)
			default:
				return fallback()
			}
		case topDone:
			// Trailing content after a complete top-level value is malformed; keep
			// the value parsed so far.
			return fallback()
		}
	}

	return fallback()
}

// closeOpenString completes a document whose final string value was cut off
// before its closing quote. q is the index of the opening quote; closers are the
// containers enclosing the string. A dangling trailing backslash is dropped so
// it cannot escape the synthetic closing quote.
func closeOpenString(raw string, q int, closers []byte) string {
	content := raw[q:]
	backslashes := 0
	for j := len(content) - 1; j >= 0 && content[j] == '\\'; j-- {
		backslashes++
	}
	if backslashes%2 == 1 {
		content = content[:len(content)-1]
	}

	var b strings.Builder
	b.WriteString(raw[:q])
	b.WriteString(content)
	b.WriteByte('"')
	appendClosers(&b, closers)
	return b.String()
}

// appendClosers writes the pending closers in reverse so the innermost container
// closes first.
func appendClosers(b *strings.Builder, closers []byte) {
	for k := len(closers) - 1; k >= 0; k-- {
		b.WriteByte(closers[k])
	}
}

// scanString returns the index just past the closing quote of the string that
// starts at the quote raw[i]. ok is false when no closing quote is found, which
// for streamed input means the string was truncated.
func scanString(raw string, i int) (int, bool) {
	for j := i + 1; j < len(raw); j++ {
		switch raw[j] {
		case '\\':
			j++ // skip the escaped character
		case '"':
			return j + 1, true
		}
	}
	return 0, false
}

// scanNumber returns the index just past a JSON number starting at raw[i]. ok is
// false when the token is truncated (ends on a sign, dot, or exponent marker at
// end of input), in which case the caller drops it.
func scanNumber(raw string, i int) (int, bool) {
	j := i
	for j < len(raw) {
		c := raw[j]
		if (c >= '0' && c <= '9') || c == '-' || c == '+' || c == '.' || c == 'e' || c == 'E' {
			j++
			continue
		}
		break
	}
	if j == i {
		return 0, false
	}
	if j < len(raw) {
		// A delimiter follows, so the number is complete.
		return j, true
	}
	// Reached end of input: complete only when it ends on a digit.
	last := raw[j-1]
	if last >= '0' && last <= '9' {
		return j, true
	}
	return 0, false
}

// scanLiteral matches one of the JSON literals true, false, or null at raw[i].
func scanLiteral(raw string, i int) (int, bool) {
	for _, literal := range []string{"true", "false", "null"} {
		if strings.HasPrefix(raw[i:], literal) {
			return i + len(literal), true
		}
	}
	return 0, false
}
