package flexeraapi

import "regexp"

// Redact removes refresh tokens, client secrets, bearer tokens, and JWTs from s.
func Redact(s string) string {
	bearer := regexp.MustCompile(`(?i)\bBearer\s+\S+`)
	s = bearer.ReplaceAllString(s, "Bearer [REDACTED]")
	jwt := regexp.MustCompile(`eyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}`)
	s = jwt.ReplaceAllString(s, "[REDACTED]")
	kv := regexp.MustCompile(
		`(?i)(refresh_token|client_secret|access_token|id_token)(\s*["']?\s*[:=]\s*["']?)([^&\s"',}]+)`,
	)
	s = kv.ReplaceAllString(s, "${1}${2}[REDACTED]")
	return s
}

// RedactError returns err with token material removed from its error string.
// Context cancellation errors are returned unchanged so errors.Is still works.
func RedactError(err error) error {
	if err == nil {
		return nil
	}
	msg := Redact(err.Error())
	if msg == err.Error() {
		return err
	}
	return &redactedError{msg: msg}
}

type redactedError struct {
	msg string
}

func (e *redactedError) Error() string {
	return e.msg
}

// StatusError is an HTTP failure from a Bill Analysis call.
type StatusError struct {
	Op     string
	Status int
	Detail string
}

func (e *StatusError) Error() string {
	if e == nil {
		return ""
	}
	return e.Op + " returned " + itoa(e.Status) + ": " + e.Detail
}

func statusErr(op string, status int, body []byte) error {
	return &StatusError{Op: op, Status: status, Detail: Redact(string(body))}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var digits [12]byte
	i := len(digits)
	for n > 0 {
		i--
		digits[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		digits[i] = '-'
	}
	return string(digits[i:])
}
