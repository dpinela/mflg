package buffer

import (
	"golang.org/x/text/unicode/norm"
	"unicode/utf8"
)

func NextCharBoundary(s string) int {
	if len(s) == 0 {
		return 0
	}
	if len(s) == 1 || (len(s) >= 2 && s[0] <= utf8.RuneSelf && s[1] <= utf8.RuneSelf) {
		return 1
	}
	return norm.NFC.NextBoundaryInString(s, true)
}
