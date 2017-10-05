package buffer

import (
	"bytes"
	"reflect"
	"testing"
)

var testData = []byte("Lorem ipsum dolor sit amet, consectetur adipiscing elit. Sed id volutpat purus. Cras suscipit, lorem id elementum varius, sem justo dignissim ligula, ac fermentum erat magna porttitor erat. Proin vitae scelerisque magna. Maecenas quis libero est. Praesent hendrerit luctus mi, eget lacinia lorem malesuada eu. Proin volutpat molestie tortor ac vestibulum. In hac habitasse platea dictumst. Sed luctus tempus fringilla. Phasellus a posuere velit. Praesent magna odio, efficitur vel pretium vel, venenatis id justo. Donec vestibulum luctus lorem. Phasellus aliquam pharetra justo vitae egestas. Donec luctus tincidunt purus vel scelerisque. Phasellus ut venenatis augue, ut consectetur nisl. Integer id magna.")

var multilineTestData = []byte(
	`Lorem ipsum dolor sit amet,
consecutur adipiscing elit.
Sed id volutpat purus.`)

func bufFromData(t *testing.T, data []byte) *Buffer {
	buf := New()
	if _, err := buf.ReadFrom(bytes.NewReader(data)); err != nil {
		t.Fatal(err)
	}
	return buf
}

func testRoundTrip(t *testing.T, data []byte) {
	buf := bufFromData(t, data)
	var outBuf bytes.Buffer
	if _, err := buf.WriteTo(&outBuf); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, outBuf.Bytes()) {
		t.Errorf("got %q, want %q", outBuf.Bytes(), data)
	}
}

func TestRoundTrip(t *testing.T)          { testRoundTrip(t, testData) }
func TestRoundTripMultiline(t *testing.T) { testRoundTrip(t, multilineTestData) }

var sliceLinesTests = []struct {
	start, end int
	want       [][]byte
}{
	{start: 1, end: 1, want: [][]byte{}},
	{start: 1, end: 2, want: [][]byte{[]byte("consecutur adipiscing elit.\n")}},
	{start: 0, end: 20, want: [][]byte{[]byte("Lorem ipsum dolor sit amet,\n"),
		[]byte("consecutur adipiscing elit.\n"), []byte("Sed id volutpat purus.")}},
}

func TestSliceLines(t *testing.T) {
	buf := bufFromData(t, multilineTestData)
	for _, tt := range sliceLinesTests {
		if lines := buf.SliceLines(tt.start, tt.end); !reflect.DeepEqual(lines, tt.want) {
			t.Errorf("SliceLines(%d, %d) = %q, want %q", tt.start, tt.end, lines, tt.want)
		}
	}
}
