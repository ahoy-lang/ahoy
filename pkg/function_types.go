package ahoy

import "strings"

// Function type strings are canonical and look like
// "fn(int,string)->void". They are produced wherever a type is parsed, so the
// code generator only has to look at one spelling.

// IsFunctionType reports whether a type string names a function value.
func IsFunctionType(t string) bool {
	return strings.HasPrefix(t, "fn(") && strings.Contains(t, ")->")
}

// ParseFunctionType splits "fn(int,string)->void" into its parameter types and
// return type. ok is false when t is not a function type.
func ParseFunctionType(t string) (params []string, ret string, ok bool) {
	if !IsFunctionType(t) {
		return nil, "", false
	}
	body := strings.TrimPrefix(t, "fn(")
	closeAt := strings.Index(body, ")")
	if closeAt < 0 {
		return nil, "", false
	}
	paramPart := body[:closeAt]
	ret = strings.TrimPrefix(body[closeAt+1:], "->")
	if paramPart != "" {
		params = strings.Split(paramPart, ",")
	}
	if ret == "" {
		ret = "void"
	}
	return params, ret, true
}

// FormatFunctionType builds the canonical function type string.
func FormatFunctionType(params []string, ret string) string {
	if ret == "" {
		ret = "void"
	}
	return "fn(" + strings.Join(params, ",") + ")->" + ret
}
