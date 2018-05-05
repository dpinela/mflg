package buffer

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

var wrapTests = []struct {
	name  string
	in    string
	width int
	out   []WrappedLine
}{
	{"Basic", wrapTestCode, 20, []WrappedLine{
		{Point{0, 0}, 0, "// wrapUntil wraps t"}, {Point{20, 0}, 20, "he source buffer unt"}, {Point{40, 0}, 40, "il the end of wrappe"}, {Point{60, 0}, 60, "d line i.\n"},
		{Point{0, 1}, 0, "func (wb *WrappedBuf"}, {Point{20, 1}, 20, "fer) wrapUntil(i int"}, {Point{40, 1}, 40, ") {"},
	}},
	{"Tabs", ":)\t\t\t\t\t:)\n", 20, []WrappedLine{{Point{0, 0}, 0, ":)\t\t\t\t"}, {Point{6, 0}, 6, "\t:)\n"}, {Point{0, 1}, 0, ""}}},
	{"CrampedTabs", "\t\t\t", 3, []WrappedLine{{Point{0, 0}, 0, "\t"}, {Point{1, 0}, 1, "\t"}, {Point{2, 0}, 2, "\t"}}},
	{"NonASCII", "áá", 1, []WrappedLine{{Point{0, 0}, 0, "á"}, {Point{1, 0}, 2, "á"}}},
}

const wrapTestCode = `// wrapUntil wraps the source buffer until the end of wrapped line i.
func (wb *WrappedBuffer) wrapUntil(i int) {`

func initWrapTest(t *testing.T, text string, width int) *WrappedBuffer {
	t.Helper()
	b := New()
	if _, err := b.ReadFrom(strings.NewReader(text)); err != nil {
		t.Fatal(err)
	}
	return NewWrapped(b, width, func(c string) int {
		if c == "\t" {
			return 4
		}
		return 1
	})
}

func (wl WrappedLine) String() string {
	return fmt.Sprintf("{%d %d %q}", wl.Start, wl.ByteStart, wl.Text)
}

func TestWrap(t *testing.T) {
	for _, tt := range wrapTests {
		t.Run(tt.name, func(t *testing.T) {
			if lines := initWrapTest(t, tt.in, tt.width).Lines(0, len(tt.out)*2); !reflect.DeepEqual(lines, tt.out) {
				t.Errorf("got %v, want %v", lines, tt.out)
			}
		})
	}
}
