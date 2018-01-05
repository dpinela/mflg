package termesc

import "testing"

var testCases = []struct {
	input  string
	output interface{}
}{
	{input: "ratoeira", output: ErrNotAMouseEvent},
	{input: "\x1B[M#!!", output: MouseEvent{
		Button: ReleaseButton, X: 0, Y: 0}},
	{input: "\x1B[35;1;1M", output: MouseEvent{
		Button: ReleaseButton, X: 0, Y: 0}},
	{input: "\x1B[32;11;16M", output: MouseEvent{
		Button: LeftButton, X: 10, Y: 15}},
	{input: "\x1B[M +0", output: MouseEvent{
		Button: LeftButton, X: 10, Y: 15}},
	{input: "\x1B[M*)\"", output: MouseEvent{
		Button: RightButton, Alt: true, X: 8, Y: 1}},
	{input: "\x1B[42;9;2M", output: MouseEvent{
		Button: RightButton, Alt: true, X: 8, Y: 1}},
	{input: "\x1B[M=\xBB!", output: MouseEvent{
		Button: MiddleButton, Alt: true, Control: true, Shift: true, X: 154, Y: 0}},
	{input: "\x1B[45;155;1M", output: MouseEvent{
		Button: MiddleButton, Alt: true, Shift: true, X: 154, Y: 0}},
	{input: "\x1B[1;6;8;4M", output: ErrNotAMouseEvent},
	{input: "\x1B[41;0;0M", output: ErrInvalidCoords},
	{input: "\x1B[M#\x1B\x1B", output: ErrInvalidCoords},
	{input: "\x1B[67;3;2M", output: MouseEvent{
		Button: NoButton, X: 2, Y: 1, Move: true}},
	{input: "\x1B[MG##", output: MouseEvent{
		Button: NoButton, Shift: true, X: 2, Y: 2, Move: true}},
	{input: "\x1B[64;15;5M", output: MouseEvent{
		Button: LeftButton, X: 14, Y: 4, Move: true}},
}

func parseResult(code string) interface{} {
	ev, err := ParseMouseEvent(code)
	if err != nil {
		return err
	}
	return ev
}

func TestParseMouseEvent(t *testing.T) {
	for _, tt := range testCases {
		res := parseResult(tt.input)
		if res != tt.output {
			t.Errorf("ParseMouseEvent(%q): got %+v, want %+v", tt.input, res, tt.output)
		}
	}
}
