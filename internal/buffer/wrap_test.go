package buffer

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

const wrapTestCode = `// wrapUntil wraps the source buffer until the end of wrapped line i.
func (wb *WrappedBuffer) wrapUntil(i int) {`

const tabsAndSmiles = ":)\t\t\t\t\t:)\n"

var wrapped0 = []WrappedLine{
	{Point{0, 0}, "// wrapUntil wraps t"}, {Point{20, 0}, "he source buffer unt"}, {Point{40, 0}, "il the end of wrappe"}, {Point{60, 0}, "d line i.\n"},
	{Point{0, 1}, "func (wb *WrappedBuf"}, {Point{20, 1}, "fer) wrapUntil(i int"}, {Point{40, 1}, ") {"},
}

var wrappedSmiles = []WrappedLine{{Point{0, 0}, ":)\t\t\t\t"}, {Point{6, 0}, "\t:)\n"}, {Point{0, 1}, ""}}

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

func (wl WrappedLine) String() string { return fmt.Sprintf("{%d %q}", wl.Start, wl.Text) }

func checkWrapResult(t *testing.T, text string, width int, expected []WrappedLine) {
	t.Helper()
	wb := initWrapTest(t, text, width)
	if lines := wb.Lines(0, len(expected)*2); !reflect.DeepEqual(lines, expected) {
		t.Errorf("got %v, want %v", lines, expected)
	}
}

func TestBasicWrap(t *testing.T) { checkWrapResult(t, wrapTestCode, 20, wrapped0) }
func TestTabWrap(t *testing.T)   { checkWrapResult(t, tabsAndSmiles, 20, wrappedSmiles) }
