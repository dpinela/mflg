package termesc

import (
	"io"
	"reflect"
	"strings"
	"testing"
)

const testInput = "\x1B[M#U7Á €50.0\x1B+25c, \x1B[Afoo\x1B[32;10;15M\x1B[Cπ"

var wantOutput = []string{"\x1B[M#U7", "Á", " ", "€", "5", "0", ".", "0", "\x1B", "+", "2", "5", "c",
	",", " ", "\x1B[A", "f", "o", "o", "\x1B[32;10;15M", "\x1B[C", "π"}

func TestInputParsing(t *testing.T) {
	c := NewConsoleReader(strings.NewReader(testInput))
	var output []string
	for {
		token, err := c.ReadToken()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		output = append(output, token)
	}
	if !reflect.DeepEqual(output, wantOutput) {
		t.Errorf("TestInputParsing: got %q, want %q", output, wantOutput)
	}
}
