package app

import "regexp"

// piiEmailRE matches RFC-5321-ish local@domain pairs aggressively. The
// trade-off is that it errs on the side of over-masking — false positives
// during an export are acceptable; leaking real addresses is not.
var piiEmailRE = regexp.MustCompile(`(?i)[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,}`)

// piiPhoneRE matches the common Korean / international shapes
// (010-1234-5678, +82-10-1234-5678, +1 (212) 555-0000). It deliberately
// requires at least one separator so it does not eat plain numbers like
// "issue 1234567890".
var piiPhoneRE = regexp.MustCompile(`\+?\d{1,4}?[\s\-.()]+\d{2,4}[\s\-.()]+\d{3,4}[\s\-.()]+\d{3,4}|\b\d{2,4}-\d{3,4}-\d{4}\b`)

// maskPII replaces email / phone fragments with placeholder tokens. The order
// matters: emails first so a phone-like substring inside an email local-part
// is not mistaken for a phone number.
func maskPII(s string) string {
	if s == "" {
		return s
	}
	s = piiEmailRE.ReplaceAllString(s, "[email]")
	s = piiPhoneRE.ReplaceAllString(s, "[phone]")
	return s
}

func maybeMaskPII(s string, mask bool) string {
	if !mask {
		return s
	}
	return maskPII(s)
}
