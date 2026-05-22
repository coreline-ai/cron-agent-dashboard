package store

import "regexp"

// piiEmailRE / piiPhoneRE mirror internal/app/workspace_io_pii.go. The store
// package is the one inserting webhook_delivery rows so the masking step has
// to live here; keeping the regex literal lets us avoid an upward import on
// app/.
var (
	piiEmailRE = regexp.MustCompile(`(?i)[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,}`)
	piiPhoneRE = regexp.MustCompile(`\+?\d{1,4}?[\s\-.()]+\d{2,4}[\s\-.()]+\d{3,4}[\s\-.()]+\d{3,4}|\b\d{2,4}-\d{3,4}-\d{4}\b`)
)

// maskPIIBytes redacts email / phone fragments inside a JSON payload byte
// slice. It operates on the raw bytes so the dispatcher can hand the result
// directly to webhook_delivery.payload_json without re-encoding.
//
// The function intentionally errs on the side of over-masking — a false
// positive turns a phone-shaped substring into "[phone]"; a false negative
// would leak real customer data. The cost of the former is operator
// confusion; the cost of the latter is a compliance incident.
func maskPIIBytes(payload []byte) []byte {
	if len(payload) == 0 {
		return payload
	}
	out := piiEmailRE.ReplaceAll(payload, []byte("[email]"))
	out = piiPhoneRE.ReplaceAll(out, []byte("[phone]"))
	return out
}
