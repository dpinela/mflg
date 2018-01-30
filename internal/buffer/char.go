package buffer

import (
	"github.com/dpinela/charseg"
	"unicode/utf8"
)

func NextCharBoundary(s string) int {
	if len(s) >= 2 && s[0] < utf8.RuneSelf && s[1] < utf8.RuneSelf {
		return 1
	}
	if len(s) < 2 {
		return len(s)
	}
	return len(charseg.FirstGraphemeCluster(s))
}

func FirstChar(s string) string {
	return charseg.FirstGraphemeCluster(s)
}

func CharCount(s string) int {
	n := 0
	for len(s) > 0 && s != "\n" {
		s = s[NextCharBoundary(s):]
		n++
	}
	return n
}
