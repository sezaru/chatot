package client

import (
	"context"
	"fmt"
	"strings"

	"go.mau.fi/whatsmeow"
)

// normalizePairingPhone strips formatting (+, spaces, dashes, parens) from a
// user-entered phone number and validates it's 7-15 digits, as required by
// whatsmeow's PairPhone.
func normalizePairingPhone(input string) (digits string, ok bool) {
	var b strings.Builder
	for _, r := range input {
		switch r {
		case '+', ' ', '-', '(', ')':
			continue
		}
		if r < '0' || r > '9' {
			return "", false
		}
		b.WriteRune(r)
	}
	digits = b.String()
	if len(digits) < 7 || len(digits) > 15 {
		return "", false
	}
	return digits, true
}

// PairPhone requests a pairing code for phone, an international number in
// any common formatting (it's normalized internally).
func (w *Whatsmeow) PairPhone(ctx context.Context, phone string) (string, error) {
	digits, ok := normalizePairingPhone(phone)
	if !ok {
		return "", fmt.Errorf("chatot/client: invalid phone number %q", phone)
	}
	return w.wa.PairPhone(ctx, digits, true, whatsmeow.PairClientChrome, "chatot")
}
