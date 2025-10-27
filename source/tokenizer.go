package main

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
	TOKEN_ASSIGN // :
	TOKEN_IS     // is (==)
	TOKEN_NOT    // not (!)
	TOKEN_OR     // or (||)
	TOKEN_AND    // and (&&)
	TOKEN_THEN   // then
	TOKEN_IF
	TOKEN_ELSE
	TOKEN_ELSEIF
	TOKEN_ANIF   // anif (alternative to elseif)
	TOKEN_SWITCH
	TOKEN_LOOP // loop (replaces while/for)
	TOKEN_IN   // in (for loop element in array)
	TOKEN_TO   // to (for loop range)
	TOKEN_FUNC
	TOKEN_RETURN
	TOKEN_IMPORT
	TOKEN_WHEN          // when (compile time)
	TOKEN_AHOY          // ahoy (print shorthand)
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
	TOKEN_BOOL_TYPE
	TOKEN_DICT_TYPE
	TOKEN_VECTOR2_TYPE
	TOKEN_COLOR_TYPE
	TOKEN_TRUE
	TOKEN_FALSE
	TOKEN_ENUM
	TOKEN_STRUCT
	TOKEN_TYPE
	TOKEN_DO
	TOKEN_BREAK
	TOKEN_SKIP
	TOKEN_DOUBLE_COLON // ::
	TOKEN_QUESTION     // ? (loop counter variable)
)

type Token struct {
	Type   TokenType
	Value  string
	Line   int
	Column int
}

func tokenize(input string) []Token {
	var tokens []Token
	lines := strings.Split(input, "\n")
	indentStack := []int{0}

	keywords := map[string]TokenType{
		"if":           TOKEN_IF,
		"else":         TOKEN_ELSE,
		"elseif":       TOKEN_ELSEIF,
		"anif":         TOKEN_ANIF,
		"switch":       TOKEN_SWITCH,
		"loop":         TOKEN_LOOP,
		"in":           TOKEN_IN,
		"to":           TOKEN_TO,
		"func":         TOKEN_FUNC,
		"return":       TOKEN_RETURN,
		"import":       TOKEN_IMPORT,
		"when":         TOKEN_WHEN,
		"ahoy":         TOKEN_AHOY,
		"is":           TOKEN_IS,
		"not":          TOKEN_NOT,
		"or":           TOKEN_OR,
		"and":          TOKEN_AND,
		"then":         TOKEN_THEN,
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
		"bool":         TOKEN_BOOL_TYPE,
		"dict":         TOKEN_DICT_TYPE,
		"vector2":      TOKEN_VECTOR2_TYPE,
		"color":        TOKEN_COLOR_TYPE,
		"true":         TOKEN_TRUE,
		"false":        TOKEN_FALSE,
		"enum":         TOKEN_ENUM,
		"struct":       TOKEN_STRUCT,
		"type":         TOKEN_TYPE,
		"do":           TOKEN_DO,
		"break":        TOKEN_BREAK,
		"skip":         TOKEN_SKIP,
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
				indent += 4
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
		if len(content) > 0 && (content[0] == '?' || content[0] == '#') {
			// Skip this line - it's a comment
			tokens = append(tokens, Token{Type: TOKEN_NEWLINE, Line: lineNum + 1})
			continue
		}
		
		for i < len(content) {
			if unicode.IsSpace(rune(content[i])) {
				i++
				continue
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

			// Strings and chars
			if content[i] == '"' || content[i] == '\'' {
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
					// Single character strings become chars
					if len(value) == 1 || (len(value) == 2 && value[0] == '\\') {
						tokenType = TOKEN_CHAR
					}
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
			case '?':
				tokens = append(tokens, Token{Type: TOKEN_QUESTION, Value: "?", Line: lineNum + 1, Column: i + 1})
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
