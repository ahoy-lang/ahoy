package ahoy

import (
	"fmt"
	"strings"
	"unicode"
)

type TokenType int

const (
	TOKEN_EOF TokenType = iota
	TOKEN_IDENTIFIER
	TOKEN_NUMBER
	TOKEN_STRING
	TOKEN_CHAR
	TOKEN_F_STRING // f"string with {vars}"
	TOKEN_ASSIGN   // :
	TOKEN_IS       // is (==)
	TOKEN_NOT      // not (!)
	TOKEN_OR       // or (||)
	TOKEN_AND      // and (&&)
	TOKEN_THEN     // then
	TOKEN_ON       // on (for switch statements)
	TOKEN_IF
	TOKEN_ELSE
	TOKEN_ELSEIF
	TOKEN_ANIF // anif (alternative to elseif)
	TOKEN_SWITCH
	TOKEN_LOOP // loop (replaces while/for)
	TOKEN_IN   // in (for loop element in array)
	TOKEN_TO   // to (for loop range)
	TOKEN_TILL // till (for loop condition)
	TOKEN_FUNC
	TOKEN_RETURN
	TOKEN_IMPORT
	TOKEN_PROGRAM       // program (package declaration)
	TOKEN_WHEN          // when (compile time)
	TOKEN_AHOY          // ahoy (print shorthand)
	TOKEN_PRINT         // print
	TOKEN_PLUS          // +
	TOKEN_MINUS         // -
	TOKEN_MULTIPLY      // *
	TOKEN_DIVIDE        // /
	TOKEN_MODULO        // %
	TOKEN_PLUS_WORD     // plus
	TOKEN_MINUS_WORD    // minus
	TOKEN_TIMES_WORD    // times
	TOKEN_DIV_WORD      // div
	TOKEN_MOD_WORD      // mod
	TOKEN_LESS          // <
	TOKEN_GREATER       // >
	TOKEN_LESS_EQUAL    // <=
	TOKEN_GREATER_EQUAL // >=
	TOKEN_LESSER_WORD   // lesser
	TOKEN_GREATER_WORD  // greater
	TOKEN_PIPE          // |
	TOKEN_LPAREN        // (
	TOKEN_RPAREN        // )
	TOKEN_LBRACE        // {
	TOKEN_RBRACE        // }
	TOKEN_LBRACKET      // [
	TOKEN_RBRACKET      // ]
	TOKEN_LANGLE        // <
	TOKEN_RANGLE        // >
	TOKEN_COMMA         // ,
	TOKEN_DOT           // .
	TOKEN_SEMICOLON     // ;
	TOKEN_NEWLINE
	TOKEN_INDENT
	TOKEN_DEDENT
	TOKEN_INT_TYPE
	TOKEN_FLOAT_TYPE
	TOKEN_STRING_TYPE
	TOKEN_CHAR_TYPE
	TOKEN_BOOL_TYPE
	TOKEN_DICT_TYPE
	TOKEN_ARRAY_TYPE
	TOKEN_VECTOR2_TYPE
	TOKEN_COLOR_TYPE
	TOKEN_TRUE
	TOKEN_FALSE
	TOKEN_ENUM
	TOKEN_STRUCT
	TOKEN_TYPE
	TOKEN_ALIAS        // alias (type alias)
	TOKEN_UNION        // union (union types)
	TOKEN_DO
	TOKEN_HALT         // halt (break from loop)
	TOKEN_NEXT         // next (continue to next iteration)
	TOKEN_ASSERT       // assert (runtime assertion)
	TOKEN_DEFER        // defer (deferred execution)
	TOKEN_DOUBLE_COLON // ::
	TOKEN_WALRUS       // := (for tuple assignment)
	TOKEN_QUESTION     // ? (loop counter variable)
	TOKEN_TERNARY      // ?? (ternary operator)
	TOKEN_EQUALS       // = (for default arguments)
	TOKEN_INFER        // infer (inferred return type)
	TOKEN_VOID         // void (no return value)
	TOKEN_END            // $ or ⚓ (block terminator)
	TOKEN_AT             // @ (function declaration prefix)
	TOKEN_PLUS_ASSIGN    // +=
	TOKEN_MINUS_ASSIGN   // -=
	TOKEN_MULTIPLY_ASSIGN // *=
	TOKEN_DIVIDE_ASSIGN   // /=
	TOKEN_MODULO_ASSIGN   // %=
	TOKEN_CARET           // ^ (pointer dereference, Pascal-style)
	TOKEN_AMPERSAND       // & (address-of, Pascal-style)
)

type Token struct {
	Type   TokenType
	Value  string
	Line   int
	Column int
}

func Tokenize(input string) []Token {
	var tokens []Token
	lines := strings.Split(input, "\n")
	indentStack := []int{0}

	keywords := map[string]TokenType{
		"if":     TOKEN_IF,
		"else":   TOKEN_ELSE,
		"elseif": TOKEN_ELSEIF,
		"anif":   TOKEN_ANIF,
		"switch": TOKEN_SWITCH,
		"loop":   TOKEN_LOOP,
		"in":     TOKEN_IN,
		"to":     TOKEN_TO,
		"till":   TOKEN_TILL,
		// "func" removed - we use :: syntax for functions
		"return":       TOKEN_RETURN,
		"import":       TOKEN_IMPORT,
		"program":      TOKEN_PROGRAM,
		"when":         TOKEN_WHEN,
		"ahoy":         TOKEN_AHOY,
		"print":        TOKEN_PRINT,
		"is":           TOKEN_IS,
		"not":          TOKEN_NOT,
		"or":           TOKEN_OR,
		"and":          TOKEN_AND,
		"then":         TOKEN_THEN,
		"on":           TOKEN_ON,
		"plus":         TOKEN_PLUS_WORD,
		"minus":        TOKEN_MINUS_WORD,
		"times":        TOKEN_TIMES_WORD,
		"div":          TOKEN_DIV_WORD,
		"mod":          TOKEN_MOD_WORD,
		"greater_than": TOKEN_GREATER_WORD,
		"lesser_than":  TOKEN_LESSER_WORD,
		"less_than":    TOKEN_LESSER_WORD,
		"int":          TOKEN_INT_TYPE,
		"float":        TOKEN_FLOAT_TYPE,
		"string":       TOKEN_STRING_TYPE,
		"char":         TOKEN_CHAR_TYPE,
		"bool":         TOKEN_BOOL_TYPE,
		"dict":         TOKEN_DICT_TYPE,
		"array":        TOKEN_ARRAY_TYPE,
		"vector2":      TOKEN_VECTOR2_TYPE,
		"color":        TOKEN_COLOR_TYPE,
		"true":         TOKEN_TRUE,
		"false":        TOKEN_FALSE,
		"enum":         TOKEN_ENUM,
		"struct":       TOKEN_STRUCT,
		"type":         TOKEN_TYPE,
		"alias":        TOKEN_ALIAS,
		"union":        TOKEN_UNION,
		"do":           TOKEN_DO,
		"halt":         TOKEN_HALT,
		"next":         TOKEN_NEXT,
		"assert":       TOKEN_ASSERT,
		"defer":        TOKEN_DEFER,
		"infer":        TOKEN_INFER,
		"void":         TOKEN_VOID,
	}

	for lineNum, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}

		// Handle indentation
		indent := 0
		for _, char := range line {
			if char == ' ' {
				indent++
			} else if char == '\t' {
				indent += 2
			} else {
				break
			}
		}

		if indent > indentStack[len(indentStack)-1] {
			indentStack = append(indentStack, indent)
			tokens = append(tokens, Token{Type: TOKEN_INDENT, Line: lineNum + 1})
		} else if indent < indentStack[len(indentStack)-1] {
			for indent < indentStack[len(indentStack)-1] {
				indentStack = indentStack[:len(indentStack)-1]
				tokens = append(tokens, Token{Type: TOKEN_DEDENT, Line: lineNum + 1})
			}
		}

		// Tokenize the line content
		content := strings.TrimSpace(line)
		i := 0

		// Check if line starts with comment
		if len(content) > 0 && content[0] == '?' {
			// Skip this line - it's a comment
			tokens = append(tokens, Token{Type: TOKEN_NEWLINE, Line: lineNum + 1})
			continue
		}

		for i < len(content) {
			// Check for ⚓ (anchor emoji - easter egg block terminator)
			if i+2 < len(content) && content[i] == 0xE2 && content[i+1] == 0x9A && content[i+2] == 0x93 {
				tokens = append(tokens, Token{Type: TOKEN_END, Value: "⚓", Line: lineNum + 1, Column: i + 1})
				i += 3 // UTF-8 anchor emoji is 3 bytes
				continue
			}
			if unicode.IsSpace(rune(content[i])) {
				i++
				continue
			}

			// Check for ?? (ternary operator) first
			if i+1 < len(content) && content[i] == '?' && content[i+1] == '?' {
				tokens = append(tokens, Token{Type: TOKEN_TERNARY, Value: "??", Line: lineNum + 1, Column: i + 1})
				i += 2
				continue
			}

			// Check for inline comment (?) - skip rest of line
			if content[i] == '?' {
				break // Skip rest of line
			}

			// Numbers
			if unicode.IsDigit(rune(content[i])) {
				start := i
				for i < len(content) && (unicode.IsDigit(rune(content[i])) || content[i] == '.') {
					i++
				}
				tokens = append(tokens, Token{
					Type:   TOKEN_NUMBER,
					Value:  content[start:i],
					Line:   lineNum + 1,
					Column: start + 1,
				})
				continue
			}

			// Strings and chars (including f-strings)
			if content[i] == '"' || content[i] == '\'' {
				// Check for f-string prefix
				isFString := false
				if i > 0 && content[i-1] == 'f' {
					// Remove the previously added 'f' identifier token
					if len(tokens) > 0 && tokens[len(tokens)-1].Type == TOKEN_IDENTIFIER && tokens[len(tokens)-1].Value == "f" {
						tokens = tokens[:len(tokens)-1]
						isFString = true
					}
				}

				quote := content[i]
				i++
				start := i
				for i < len(content) && content[i] != quote {
					if content[i] == '\\' {
						i++ // Skip escaped character
					}
					i++
				}
				if i < len(content) {
					value := content[start:i]
					tokenType := TOKEN_STRING

					if isFString {
						tokenType = TOKEN_F_STRING
					}
					// Note: Removed automatic CHAR conversion to fix dictionary keys
					// Single-character strings remain as STRING tokens

					tokens = append(tokens, Token{
						Type:   tokenType,
						Value:  value,
						Line:   lineNum + 1,
						Column: start,
					})
					i++ // Skip closing quote
				}
				continue
			}

			// Identifiers and keywords
			if unicode.IsLetter(rune(content[i])) || content[i] == '_' {
				start := i
				for i < len(content) && (unicode.IsLetter(rune(content[i])) || unicode.IsDigit(rune(content[i])) || content[i] == '_') {
					i++
				}
				word := content[start:i]
				if tokenType, exists := keywords[word]; exists {
					tokens = append(tokens, Token{
						Type:   tokenType,
						Value:  word,
						Line:   lineNum + 1,
						Column: start + 1,
					})
				} else {
					tokens = append(tokens, Token{
						Type:   TOKEN_IDENTIFIER,
						Value:  word,
						Line:   lineNum + 1,
						Column: start + 1,
					})
				}
				continue
			}

			// Two-character operators
			if i+1 < len(content) {
				twoChar := content[i : i+2]
				switch twoChar {
				case ":=":
					tokens = append(tokens, Token{Type: TOKEN_WALRUS, Value: ":=", Line: lineNum + 1, Column: i + 1})
					i += 2
					continue
				case "::":
					tokens = append(tokens, Token{Type: TOKEN_DOUBLE_COLON, Value: "::", Line: lineNum + 1, Column: i + 1})
					i += 2
					continue
				case "<=":
					tokens = append(tokens, Token{Type: TOKEN_LESS_EQUAL, Value: "<=", Line: lineNum + 1, Column: i + 1})
					i += 2
					continue
				case ">=":
					tokens = append(tokens, Token{Type: TOKEN_GREATER_EQUAL, Value: ">=", Line: lineNum + 1, Column: i + 1})
					i += 2
					continue
				case "+=":
					tokens = append(tokens, Token{Type: TOKEN_PLUS_ASSIGN, Value: "+=", Line: lineNum + 1, Column: i + 1})
					i += 2
					continue
				case "-=":
					tokens = append(tokens, Token{Type: TOKEN_MINUS_ASSIGN, Value: "-=", Line: lineNum + 1, Column: i + 1})
					i += 2
					continue
				case "*=":
					tokens = append(tokens, Token{Type: TOKEN_MULTIPLY_ASSIGN, Value: "*=", Line: lineNum + 1, Column: i + 1})
					i += 2
					continue
				case "/=":
					tokens = append(tokens, Token{Type: TOKEN_DIVIDE_ASSIGN, Value: "/=", Line: lineNum + 1, Column: i + 1})
					i += 2
					continue
				case "%=":
					tokens = append(tokens, Token{Type: TOKEN_MODULO_ASSIGN, Value: "%=", Line: lineNum + 1, Column: i + 1})
					i += 2
					continue
				}
			}

			// Single-character operators
			switch content[i] {
			case '+':
				tokens = append(tokens, Token{Type: TOKEN_PLUS, Value: "+", Line: lineNum + 1, Column: i + 1})
			case '-':
				tokens = append(tokens, Token{Type: TOKEN_MINUS, Value: "-", Line: lineNum + 1, Column: i + 1})
			case '*':
				tokens = append(tokens, Token{Type: TOKEN_MULTIPLY, Value: "*", Line: lineNum + 1, Column: i + 1})
			case '/':
				tokens = append(tokens, Token{Type: TOKEN_DIVIDE, Value: "/", Line: lineNum + 1, Column: i + 1})
			case '%':
				tokens = append(tokens, Token{Type: TOKEN_MODULO, Value: "%", Line: lineNum + 1, Column: i + 1})
			case '<':
				tokens = append(tokens, Token{Type: TOKEN_LANGLE, Value: "<", Line: lineNum + 1, Column: i + 1})
			case '>':
				tokens = append(tokens, Token{Type: TOKEN_RANGLE, Value: ">", Line: lineNum + 1, Column: i + 1})
			case ':':
				tokens = append(tokens, Token{Type: TOKEN_ASSIGN, Value: ":", Line: lineNum + 1, Column: i + 1})
			case '|':
				tokens = append(tokens, Token{Type: TOKEN_PIPE, Value: "|", Line: lineNum + 1, Column: i + 1})
			case '(':
				tokens = append(tokens, Token{Type: TOKEN_LPAREN, Value: "(", Line: lineNum + 1, Column: i + 1})
			case ')':
				tokens = append(tokens, Token{Type: TOKEN_RPAREN, Value: ")", Line: lineNum + 1, Column: i + 1})
			case '{':
				tokens = append(tokens, Token{Type: TOKEN_LBRACE, Value: "{", Line: lineNum + 1, Column: i + 1})
			case '}':
				tokens = append(tokens, Token{Type: TOKEN_RBRACE, Value: "}", Line: lineNum + 1, Column: i + 1})
			case '[':
				tokens = append(tokens, Token{Type: TOKEN_LBRACKET, Value: "[", Line: lineNum + 1, Column: i + 1})
			case ']':
				tokens = append(tokens, Token{Type: TOKEN_RBRACKET, Value: "]", Line: lineNum + 1, Column: i + 1})
			case ',':
				tokens = append(tokens, Token{Type: TOKEN_COMMA, Value: ",", Line: lineNum + 1, Column: i + 1})
			case '.':
				tokens = append(tokens, Token{Type: TOKEN_DOT, Value: ".", Line: lineNum + 1, Column: i + 1})
			case ';':
				tokens = append(tokens, Token{Type: TOKEN_SEMICOLON, Value: ";", Line: lineNum + 1, Column: i + 1})
			case '=':
				tokens = append(tokens, Token{Type: TOKEN_EQUALS, Value: "=", Line: lineNum + 1, Column: i + 1})
			case '@':
				tokens = append(tokens, Token{Type: TOKEN_AT, Value: "@", Line: lineNum + 1, Column: i + 1})
			case '^':
				tokens = append(tokens, Token{Type: TOKEN_CARET, Value: "^", Line: lineNum + 1, Column: i + 1})
			case '&':
				tokens = append(tokens, Token{Type: TOKEN_AMPERSAND, Value: "&", Line: lineNum + 1, Column: i + 1})
			case '$':
				// Check for $#N syntax (multiple block closures)
				if i+1 < len(content) && content[i+1] == '#' {
					i += 2 // skip $ and #
					// Read the number
					numStart := i
					for i < len(content) && content[i] >= '0' && content[i] <= '9' {
						i++
					}
					if i > numStart {
						countStr := content[numStart:i]
						tokens = append(tokens, Token{Type: TOKEN_END, Value: "$#" + countStr, Line: lineNum + 1, Column: numStart - 1})
						i-- // will be incremented at end of loop
					} else {
						// No number after #, treat as regular $
						tokens = append(tokens, Token{Type: TOKEN_END, Value: "$", Line: lineNum + 1, Column: i - 1})
						i-- // back up since we advanced past #
					}
				} else {
					tokens = append(tokens, Token{Type: TOKEN_END, Value: "$", Line: lineNum + 1, Column: i + 1})
				}
			// Remove TOKEN_QUESTION case - now handled as comment marker above
			default:
				fmt.Printf("Unknown character: %c at line %d, column %d\n", content[i], lineNum+1, i+1)
			}
			i++
		}

		tokens = append(tokens, Token{Type: TOKEN_NEWLINE, Line: lineNum + 1})
	}

	// Add final dedents
	for len(indentStack) > 1 {
		indentStack = indentStack[:len(indentStack)-1]
		tokens = append(tokens, Token{Type: TOKEN_DEDENT})
	}

	tokens = append(tokens, Token{Type: TOKEN_EOF})
	return tokens
}
