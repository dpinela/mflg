package highlight

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/dpinela/mflg/internal/color"
)

type styledDoc []StyledRegion

func (st styledDoc) String() string {
	sb := strings.Builder{}
	for _, piece := range st {
		fmt.Fprintf(&sb, "style=%+v text=(%d)[%d:%d]\n", *piece.Style, piece.Line, piece.Start, piece.End)
	}
	return sb.String()
}

type testSource []string

func (ts testSource) SliceLines(i, j int) []string { return ts[i:j] }

var testCases = []struct {
	lang string
	in   testSource
	out  []StyledRegion
}{
	{"go", gocode, goHighlights},
	{"c", gocode, goHighlights[:len(goHighlights)-1]},
	{"json", jsoncode, jsonHighlights},
}

// This code is used to test both Go and C highlighting, since they're similar enough.
var gocode = testSource{
	"/* Package fish implements fish-related services.\n",
	" It cannot be used on land. */\n",
	"package fish\n",
	"\n",
	"var All = []string{\n",
	`	"//Shark", //whee` + "\n",
	`	"Carp /*",` + "\n",
	`	"Tuna \"*/",` + "\n",
	`	"\\", name,` + "\n",
	"}\n",
	`var R = []rune{'\\', 25, 10}` + "\n",
	"const B = `bass\\`", // not used in C highlighting
}

var testPalette = &Palette{
	Comment: Style{Foreground: &color.Color{0, 200, 0}},
	String:  Style{Foreground: &color.Color{0, 0, 200}},
}

var goHighlights = []StyledRegion{
	{Line: 0, Start: 0, End: len(gocode[0]), Style: &testPalette.Comment},
	{Line: 1, Start: 0, End: len(gocode[1]) - 1, Style: &testPalette.Comment},
	{Line: 5, Start: 1, End: 10, Style: &testPalette.String},
	{Line: 5, Start: 12, End: len(gocode[5]), Style: &testPalette.Comment},
	{Line: 6, Start: 1, End: 10, Style: &testPalette.String},
	{Line: 7, Start: 1, End: 12, Style: &testPalette.String},
	{Line: 8, Start: 1, End: 5, Style: &testPalette.String},
	{Line: 10, Start: 15, End: 19, Style: &testPalette.String},
	{Line: 11, Start: 10, End: 17, Style: &testPalette.String},
}

func TestHighlight(t *testing.T) {
	for _, tt := range testCases {
		t.Run(tt.lang, func(t *testing.T) {
			if got := Language(tt.lang, tt.in, testPalette).Regions(0, len(tt.in)); !reflect.DeepEqual(got, tt.out) {
				t.Errorf("got:\n%+v\nwant:\n%+v", styledDoc(got), styledDoc(tt.out))
			}
		})
	}
}

var jsoncode = testSource{
	"// not really valid JSON\n",
	`{"\"but\"": 'we try anyway'}`,
}

var jsonHighlights = []StyledRegion{
	{Line: 1, Start: 1, End: 10, Style: &testPalette.String},
}
