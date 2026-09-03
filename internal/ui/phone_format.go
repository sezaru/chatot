package ui

import "strings"

// formatPhoneDisplay renders an E.164 number (digits, with or without a
// leading +) the way WhatsApp prints it: "+55 48 8807-3648", "+1 415 555
// 0132", "+351 912 004 118". Unknown country codes fall back to "+" and the
// digits grouped in threes from the right, which still reads better than a
// bare 13-digit run. Anything that is not a number comes back unchanged.
func formatPhoneDisplay(raw string) string {
	digits := strings.TrimPrefix(strings.TrimSpace(raw), "+")
	if digits == "" {
		return raw
	}
	for _, r := range digits {
		if r < '0' || r > '9' {
			return raw
		}
	}
	for _, cc := range phoneCountryFormats {
		if !strings.HasPrefix(digits, cc.code) {
			continue
		}
		rest := digits[len(cc.code):]
		if grouped := cc.group(rest); grouped != "" {
			return "+" + cc.code + " " + grouped
		}
	}
	return "+" + groupFromRight(digits, 3)
}

// phoneCountryFormat groups a country's national number.
type phoneCountryFormat struct {
	code  string
	group func(rest string) string
}

// phoneCountryFormats is ordered longest code first so "351" wins over
// "35" and "55" over "5".
var phoneCountryFormats = []phoneCountryFormat{
	{"351", func(r string) string { return splitAt(r, 3, 3, 3) }}, // Portugal: 912 004 118
	{"44", func(r string) string { return splitAt(r, 4, 6) }},     // UK: 7911 123456
	{"49", func(r string) string { return groupFromRight(r, 4) }}, // Germany
	{"55", func(r string) string { // Brazil: AA NNNNN-NNNN / AA NNNN-NNNN
		if len(r) != 10 && len(r) != 11 {
			return ""
		}
		return r[:2] + " " + r[2:len(r)-4] + "-" + r[len(r)-4:]
	}},
	{"33", func(r string) string { return splitAt(r, 1, 2, 2, 2, 2) }}, // France
	{"34", func(r string) string { return splitAt(r, 3, 3, 3) }},       // Spain
	{"39", func(r string) string { return splitAt(r, 3, 3, 4) }},       // Italy
	{"1", func(r string) string { return splitAt(r, 3, 3, 4) }},        // NANP: 415 555 0132
}

// splitAt spaces rest into groups of the given sizes; "" when the digit
// count doesn't add up (the caller then falls back).
func splitAt(rest string, sizes ...int) string {
	total := 0
	for _, n := range sizes {
		total += n
	}
	if len(rest) != total {
		return ""
	}
	parts := make([]string, 0, len(sizes))
	for _, n := range sizes {
		parts = append(parts, rest[:n])
		rest = rest[n:]
	}
	return strings.Join(parts, " ")
}

// groupFromRight spaces digits in groups of n counting from the right.
func groupFromRight(digits string, n int) string {
	var parts []string
	for len(digits) > n {
		parts = append([]string{digits[len(digits)-n:]}, parts...)
		digits = digits[:len(digits)-n]
	}
	if digits != "" {
		parts = append([]string{digits}, parts...)
	}
	return strings.Join(parts, " ")
}
