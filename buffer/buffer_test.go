package buffer

import (
	"bytes"
	"testing"
)

var testData = []byte("Lorem ipsum dolor sit amet, consectetur adipiscing elit. Sed id volutpat purus. Cras suscipit, lorem id elementum varius, sem justo dignissim ligula, ac fermentum erat magna porttitor erat. Proin vitae scelerisque magna. Maecenas quis libero est. Praesent hendrerit luctus mi, eget lacinia lorem malesuada eu. Proin volutpat molestie tortor ac vestibulum. In hac habitasse platea dictumst. Sed luctus tempus fringilla. Phasellus a posuere velit. Praesent magna odio, efficitur vel pretium vel, venenatis id justo. Donec vestibulum luctus lorem. Phasellus aliquam pharetra justo vitae egestas. Donec luctus tincidunt purus vel scelerisque. Phasellus ut venenatis augue, ut consectetur nisl. Integer id magna.")

func testRoundTrip(initialSize int, t *testing.T) {
	buf := New(initialSize)
	if _, err := buf.ReadFrom(bytes.NewReader(testData)); err != nil {
		t.Fatal(err)
	}
	var outBuf bytes.Buffer
	if _, err := buf.WriteTo(&outBuf); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(testData, outBuf.Bytes()) {
		t.Errorf("got %q, want %q", outBuf.Bytes(), testData)
	}
}

func TestRoundTripFrom0(t *testing.T)   { testRoundTrip(0, t) }
func TestRoundTripFrom256(t *testing.T) { testRoundTrip(256, t) }
func TestRoundTripFromLen(t *testing.T) { testRoundTrip(len(testData), t) }
