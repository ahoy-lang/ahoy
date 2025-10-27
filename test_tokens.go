package main

import (
"fmt"
"os"
)

func main() {
content := `color enum:
    RED
    GREEN
    BLUE

ahoy|"Enum test"|`

tokens := tokenize(content)
for _, tok := range tokens {
fmt.Printf("Line %d: Type=%d Value='%s'\n", tok.Line, tok.Type, tok.Value)
}
}
