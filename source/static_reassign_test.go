package main

import (
	"strings"
	"testing"

	ahoy "ahoy"
)

// Conditional static reassignment over an OWNED binding (tree_panel
// `eye_name`): the taken path must duplicate so tracking stays valid,
// and scope exit must still free on the not-taken path. Neither path may
// leak or free non-heap.
func TestConditionalStaticOverOwned(t *testing.T) {
	src := `program p
@ main ||:
	loop i:0 to 2 do
		eye_name: string = "icon_eye"
		if i is 1 then
			eye_name = "icon_eye_off"
		$
		n: eye_name.byte_len()
		print(n)
	$
	print("done")
$
`
	ast := ahoy.ParseWithPath(ahoy.Tokenize(src), "test.ahoy")
	out := GenerateCWithFilename(ast, "test.ahoy")
	if !strings.Contains(out, `eye_name = strdup("icon_eye_off")`) {
		t.Errorf("conditional static reassignment over owned binding must duplicate")
	}
	if !strings.Contains(out, "free(eye_name)") {
		t.Errorf("owned binding must still be freed at scope exit")
	}
}
