// Package codeanalyzer provides single-pass JavaScript/TypeScript token analysis.
package codeanalyzer

import "strings"

// Result contains extracted tokens and derived flags from a single code scan.
type Result struct {
	identifiers map[string]struct{}
	calls       map[string]struct{}

	Complexity int
	HasAsync   bool
	HasAwait   bool
}

// Analyze performs a single-pass scan while skipping strings/comments.
func Analyze(code string) Result {
	result := Result{
		identifiers: make(map[string]struct{}),
		calls:       make(map[string]struct{}),
	}

	const (
		stateCode = iota
		stateLineComment
		stateBlockComment
		stateSingleQuote
		stateDoubleQuote
		stateTemplateText
		stateTemplateExpr
	)

	n := len(code)
	state := stateCode
	resumeState := stateCode
	templateResumeStack := make([]int, 0, 2)
	templateExprDepthStack := make([]int, 0, 2)

	for i := 0; i < n; {
		switch state {
		case stateCode, stateTemplateExpr:
			c := code[i]

			// End of current template expression.
			if state == stateTemplateExpr {
				switch c {
				case '{':
					top := len(templateExprDepthStack) - 1
					if top >= 0 {
						templateExprDepthStack[top]++
					}
					i++
					continue
				case '}':
					top := len(templateExprDepthStack) - 1
					if top >= 0 {
						templateExprDepthStack[top]--
						if templateExprDepthStack[top] == 0 {
							templateExprDepthStack = templateExprDepthStack[:top]
							state = stateTemplateText
							i++
							continue
						}
					}
					i++
					continue
				}
			}

			// Comments.
			if c == '/' && i+1 < n {
				switch code[i+1] {
				case '/':
					resumeState = state
					state = stateLineComment
					i += 2
					continue
				case '*':
					resumeState = state
					state = stateBlockComment
					i += 2
					continue
				}
			}

			// Strings.
			switch c {
			case '\'':
				resumeState = state
				state = stateSingleQuote
				i++
				continue
			case '"':
				resumeState = state
				state = stateDoubleQuote
				i++
				continue
			case '`':
				templateResumeStack = append(templateResumeStack, state)
				state = stateTemplateText
				i++
				continue
			case '=':
				if i+1 < n && code[i+1] == '>' {
					result.Complexity++
					i += 2
					continue
				}
			}

			// Identifiers / keywords.
			if isIdentifierStart(c) {
				start := i
				i++
				for i < n && isIdentifierPart(code[i]) {
					i++
				}

				ident := code[start:i]
				result.identifiers[ident] = struct{}{}

				switch ident {
				case "async":
					result.HasAsync = true
				case "await":
					result.HasAwait = true
				case "for", "while", "do", "if", "switch", "function":
					result.Complexity++
				}

				if nextChar, ok := nextSignificantChar(code, i); ok && nextChar == '(' {
					result.calls[ident] = struct{}{}
				}
				continue
			}

			i++

		case stateLineComment:
			if code[i] == '\n' {
				state = resumeState
			}
			i++

		case stateBlockComment:
			if code[i] == '*' && i+1 < n && code[i+1] == '/' {
				state = resumeState
				i += 2
				continue
			}
			i++

		case stateSingleQuote:
			if code[i] == '\\' {
				i += 2
				continue
			}
			if code[i] == '\'' {
				state = resumeState
			}
			i++

		case stateDoubleQuote:
			if code[i] == '\\' {
				i += 2
				continue
			}
			if code[i] == '"' {
				state = resumeState
			}
			i++

		case stateTemplateText:
			c := code[i]
			if c == '\\' {
				i += 2
				continue
			}
			if c == '`' {
				stackTop := len(templateResumeStack) - 1
				if stackTop >= 0 {
					state = templateResumeStack[stackTop]
					templateResumeStack = templateResumeStack[:stackTop]
				} else {
					state = stateCode
				}
				i++
				continue
			}
			if c == '$' && i+1 < n && code[i+1] == '{' {
				templateExprDepthStack = append(templateExprDepthStack, 1)
				state = stateTemplateExpr
				i += 2
				continue
			}
			i++
		}
	}

	return result
}

// HasIdentifier reports whether identifier appears as a token.
func (r Result) HasIdentifier(name string) bool {
	_, ok := r.identifiers[name]
	return ok
}

// HasCall reports whether identifier appears as a direct call target.
func (r Result) HasCall(name string) bool {
	_, ok := r.calls[name]
	return ok
}

func isIdentifierStart(c byte) bool {
	return c == '_' || c == '$' || ('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z')
}

func isIdentifierPart(c byte) bool {
	return isIdentifierStart(c) || ('0' <= c && c <= '9')
}

func nextSignificantChar(code string, idx int) (byte, bool) {
	n := len(code)
	for idx < n {
		c := code[idx]
		if strings.ContainsRune(" \t\r\n", rune(c)) {
			idx++
			continue
		}
		if c == '/' && idx+1 < n {
			switch code[idx+1] {
			case '/':
				idx += 2
				for idx < n && code[idx] != '\n' {
					idx++
				}
				continue
			case '*':
				idx += 2
				for idx+1 < n && (code[idx] != '*' || code[idx+1] != '/') {
					idx++
				}
				if idx+1 < n {
					idx += 2
				}
				continue
			}
		}
		return c, true
	}
	return 0, false
}
