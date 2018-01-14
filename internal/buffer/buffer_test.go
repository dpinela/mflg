package buffer

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

var testData = "Lorem ipsum dolor sit amet, consectetur adipiscing elit. Sed id volutpat purus. Cras suscipit, lorem id elementum varius, sem justo dignissim ligula, ac fermentum erat magna porttitor erat. Proin vitae scelerisque magna. Maecenas quis libero est. Praesent hendrerit luctus mi, eget lacinia lorem malesuada eu. Proin volutpat molestie tortor ac vestibulum. In hac habitasse platea dictumst. Sed luctus tempus fringilla. Phasellus a posuere velit. Praesent magna odio, efficitur vel pretium vel, venenatis id justo. Donec vestibulum luctus lorem. Phasellus aliquam pharetra justo vitae egestas. Donec luctus tincidunt purus vel scelerisque. Phasellus ut venenatis augue, ut consectetur nisl. Integer id magna."

var multilineTestData = `Lorem ipsum dolor sit amet,
consecutur adipiscing elit.
Sed id volutpat purus.`

var multilineDataAfterInsert = `LoremDING
TEXT
FOO ipsum dolor sit amet,
consecutur adipiscing elit.
Sed id volutpat purus.`

var multilineDataAfterInsertSL = `LoremDING ipsum dolor sit amet,
consecutur adipiscing elit.
Sed id volutpat purus.`

func bufFromData(t *testing.T, data string) *Buffer {
	t.Helper()
	buf := New()
	if _, err := buf.ReadFrom(strings.NewReader(data)); err != nil {
		t.Fatal(err)
	}
	return buf
}

func testRoundTrip(t *testing.T, data string) {
	testContent(t, bufFromData(t, data), data)
}

func testContent(t *testing.T, buf *Buffer, data string) {
	var outBuf bytes.Buffer
	if _, err := buf.WriteTo(&outBuf); err != nil {
		t.Fatal(err)
	}
	if s := outBuf.String(); data != s {
		t.Errorf("got %q, want %q", s, data)
	}
}

func TestRoundTrip(t *testing.T)          { testRoundTrip(t, testData) }
func TestRoundTripMultiline(t *testing.T) { testRoundTrip(t, multilineTestData) }

var sliceLinesTests = []struct {
	start, end int
	want       []string
}{
	{start: 1, end: 1, want: []string{}},
	{start: 1, end: 2, want: []string{"consecutur adipiscing elit.\n"}},
	{start: 0, end: 20, want: []string{"Lorem ipsum dolor sit amet,\n",
		"consecutur adipiscing elit.\n", "Sed id volutpat purus."}},
}

func TestSliceLines(t *testing.T) {
	buf := bufFromData(t, multilineTestData)
	for _, tt := range sliceLinesTests {
		if lines := buf.SliceLines(tt.start, tt.end); !reflect.DeepEqual(lines, tt.want) {
			t.Errorf("SliceLines(%d, %d) = %q, want %q", tt.start, tt.end, lines, tt.want)
		}
	}
}

func TestInsertMultiLine(t *testing.T) {
	buf := bufFromData(t, multilineTestData)
	n := buf.LineCount()
	wantN := n + 2
	buf.Insert("DING\nTEXT\nFOO", Point{5, 0})
	testContent(t, buf, multilineDataAfterInsert)
	if buf.LineCount() != wantN {
		t.Errorf("after insert: got %d lines, want %d", buf.LineCount(), wantN)
	}
}

func TestInsertSingleLine(t *testing.T) {
	buf := bufFromData(t, multilineTestData)
	buf.Insert("DING", Point{5, 0})
	testContent(t, buf, multilineDataAfterInsertSL)
}

var indentTests = []struct {
	name, in    string
	indentLevel int
}{
	{name: "Empty", in: "", indentLevel: IndentTabs},
	{name: "NoIndent", in: "foo\nbar\nblam\n", indentLevel: IndentTabs},
	{name: "Tabs", in: `#include <stdio.h>
int main() {
	puts("OK.");
	return 0;
}`, indentLevel: IndentTabs},
	{name: "Spaces", in: `import re
def adder(x):
  def f(y):
    return x + y
  return f
  
print(adder(9)(9))`, indentLevel: 2},
	{name: "Mixed", in: `package badindent
func A(x string) string {
    if x == "dog" {
    	return "cat"
    }
    if x == "dogs" {
        return "cats"
    }
	return "dog"
}`, indentLevel: 4},
}

func TestIndentAutodetect(t *testing.T) {
	for _, tt := range indentTests {
		t.Run(tt.name, func(t *testing.T) {
			if level := bufFromData(t, tt.in).IndentType(); level != tt.indentLevel {
				t.Errorf("got indent=%d, want=%d", level, tt.indentLevel)
			}
		})
	}
}

const wordBoundsBracketsTest = "teach(a)[man]->to {fish,now}"

var wordBoundsTests = []struct {
	in   string
	p    Point
	want Range
}{
	// Points within words
	{in: multilineTestData, p: Point{7, 0}, want: Range{Point{6, 0}, Point{11, 0}}},
	{in: wordBoundsBracketsTest, p: Point{0, 0}, want: Range{Point{0, 0}, Point{5, 0}}},
	{in: wordBoundsBracketsTest, p: Point{6, 0}, want: Range{Point{6, 0}, Point{7, 0}}},
	{in: wordBoundsBracketsTest, p: Point{11, 0}, want: Range{Point{9, 0}, Point{12, 0}}},
	{in: wordBoundsBracketsTest, p: Point{15, 0}, want: Range{Point{15, 0}, Point{17, 0}}},
	{in: wordBoundsBracketsTest, p: Point{20, 0}, want: Range{Point{19, 0}, Point{23, 0}}},
	{in: wordBoundsBracketsTest, p: Point{25, 0}, want: Range{Point{24, 0}, Point{27, 0}}},

	// Points outside of words
	{in: multilineTestData, p: Point{5, 0}, want: Range{Point{5, 0}, Point{5, 0}}},
	{in: wordBoundsBracketsTest, p: Point{5, 0}, want: Range{Point{5, 0}, Point{5, 0}}},
	{in: wordBoundsBracketsTest, p: Point{8, 0}, want: Range{Point{8, 0}, Point{8, 0}}},
	{in: wordBoundsBracketsTest, p: Point{12, 0}, want: Range{Point{12, 0}, Point{12, 0}}},
	{in: wordBoundsBracketsTest, p: Point{18, 0}, want: Range{Point{18, 0}, Point{18, 0}}},
	{in: wordBoundsBracketsTest, p: Point{23, 0}, want: Range{Point{23, 0}, Point{23, 0}}},
	{in: wordBoundsBracketsTest, p: Point{27, 0}, want: Range{Point{27, 0}, Point{27, 0}}},
}

func TestWordBounds(t *testing.T) {
	for _, tt := range wordBoundsTests {
		if r := bufFromData(t, tt.in).WordBoundsAt(tt.p); r != tt.want {
			t.Errorf("word bounds at %v in %q...: got %v, want %v", tt.p, tt.in[:20], r, tt.want)
		}
	}
}
