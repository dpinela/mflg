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

func goHighlightList(pal *Palette) []StyledRegion {
	return []StyledRegion{
		{Line: 0, Start: 0, End: len(gocode[0]), Style: &pal.Comment},
		{Line: 1, Start: 0, End: len(gocode[1]) - 1, Style: &pal.Comment},
		{Line: 5, Start: 1, End: 10, Style: &pal.String},
		{Line: 5, Start: 12, End: len(gocode[5]), Style: &pal.Comment},
		{Line: 6, Start: 1, End: 10, Style: &pal.String},
		{Line: 7, Start: 1, End: 12, Style: &pal.String},
		{Line: 8, Start: 1, End: 5, Style: &pal.String},
		{Line: 10, Start: 15, End: 19, Style: &pal.String},
		{Line: 11, Start: 10, End: 17, Style: &pal.String},
	}
}

func TestGoStyle(t *testing.T) {
	want := goHighlightList(testPalette)
	h := Language("go", gocode, testPalette)
	if got := h.Regions(0, len(gocode)); !reflect.DeepEqual(got, want) {
		t.Errorf("got:\n%+v\nwant:\n%+v", styledDoc(got), styledDoc(want))
	}
}

func TestCStyle(t *testing.T) {
	want := goHighlightList(testPalette)
	h := Language("c", gocode, testPalette)
	want = want[:len(want)-1]
	if got := h.Regions(0, len(gocode)); !reflect.DeepEqual(got, want) {
		t.Errorf("got:\n%+v\nwant:\n%+v", styledDoc(got), styledDoc(want))
	}
}
