// SPDX-License-Identifier: Apache-2.0

// Package notify selects, renders and dispatches guardian alerts through a
// pluggable Channel. The mock channel is the CI/demo default; output derived
// from it must always say "would send", never "sent".
package notify

import (
	"fmt"
)

// GSM 03.38 default alphabet: every character costs 1 septet; characters in
// the extension table cost 2 (ESC + char). Anything else cannot be encoded
// as GSM-7 (the carrier would silently switch to UCS-2 and halve the
// message budget), so we reject it outright.

var gsm7Basic = map[rune]struct{}{}

var gsm7Extension = map[rune]struct{}{
	'\f': {}, '^': {}, '{': {}, '}': {}, '\\': {}, '[': {}, ']': {}, '~': {}, '|': {}, '€': {},
}

func init() {
	const basic = "@£$¥èéùìòÇ\nØø\rÅåΔ_ΦΓΛΩΠΨΣΘΞÆæßÉ !\"#¤%&'()*+,-./0123456789:;<=>?" +
		"¡ABCDEFGHIJKLMNOPQRSTUVWXYZÄÖÑÜ§¿abcdefghijklmnopqrstuvwxyzäöñüà"
	for _, r := range basic {
		gsm7Basic[r] = struct{}{}
	}
}

// SeptetLength returns the GSM-7 septet count of s, or an error if s
// contains any character outside the GSM 03.38 default + extension tables.
func SeptetLength(s string) (int, error) {
	n := 0
	for _, r := range s {
		switch {
		case hasBasic(r):
			n++
		case hasExtension(r):
			n += 2
		default:
			return 0, fmt.Errorf("notify: character %q is not GSM-7 encodable", r)
		}
	}
	return n, nil
}

func hasBasic(r rune) bool {
	_, ok := gsm7Basic[r]
	return ok
}

func hasExtension(r rune) bool {
	_, ok := gsm7Extension[r]
	return ok
}
