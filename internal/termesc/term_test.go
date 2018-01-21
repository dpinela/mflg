package termesc

import (
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"
)

const testInput = "\x1B[M#U7Á €50.0\x1B+25c, \x1B[Afoo\x1B[32;10;15M\x1B[Cπ\x1bb\x1bf\x1b\x1b[D"

var wantOutput = []string{"\x1B[M#U7", "Á", " ", "€", "5", "0", ".", "0", "\x1B", "+", "2", "5", "c",
	",", " ", "\x1B[A", "f", "o", "o", "\x1B[32;10;15M", "\x1B[C", "π", "\x1bb", "\x1bf", "\x1b\x1b[D"}

func TestInputParsing(t *testing.T) {
	c := NewConsoleReader(strings.NewReader(testInput))
	output := getAllOutput(t, c)
	if !reflect.DeepEqual(output, wantOutput) {
		t.Errorf("TestInputParsing: got %q, want %q", output, wantOutput)
	}
}

func getAllOutput(t *testing.T, r *ConsoleReader) (output []string) {
	for {
		token, err := r.ReadToken()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		output = append(output, token)
	}
	return
}

var wantOutput2 = []string{"\x1B", "[", "\x1B", "f"}

func printWithDelay(w io.Writer, delay time.Duration, s1, s2 string) {
	fmt.Fprint(w, s1)
	time.Sleep(delay)
	fmt.Fprint(w, s2)
}

func TestStandaloneEsc(t *testing.T) {
	r, w := io.Pipe()
	go func() {
		// We don't check for the write errors here; any failure will be picked up by the read end
		printWithDelay(w, 50*time.Millisecond, "\x1b", "[")
		printWithDelay(w, 50*time.Millisecond, "\x1b", "f")
		w.Close()
	}()
	c := NewConsoleReader(r)
	output := getAllOutput(t, c)
	if !reflect.DeepEqual(output, wantOutput2) {
		t.Errorf("TestStandaloneEsc: got %q, want %q", output, wantOutput2)
	}
}
