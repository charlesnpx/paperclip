package policy

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/charlesnpx/paperclip/internal/domain"
)

type Denial struct {
	Field string
	Rule  string
}

func (d Denial) Error() string {
	return fmt.Sprintf("policy denied field %s by rule %s", d.Field, d.Rule)
}

type Scanner struct {
	rules []rule
}

type rule struct {
	name string
	re   *regexp.Regexp
}

func DefaultScanner() Scanner {
	return Scanner{rules: []rule{
		{name: "private-key-block", re: regexp.MustCompile(`(?is)-----BEGIN [A-Z ]*PRIVATE KEY-----`)},
		{name: "url-userinfo-credential", re: regexp.MustCompile(`(?i)\b[a-z][a-z0-9+.-]*://[^\s/@]+@`)},
		{name: "bearer-token", re: regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9._~+/=-]{12,}`)},
		{name: "aws-access-key", re: regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`)},
		{name: "github-token", re: regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9_]{20,}\b`)},
		{name: "openai-token", re: regexp.MustCompile(`\bsk-[A-Za-z0-9]{20,}\b`)},
	}}
}

func (s Scanner) ScanFields(fields map[string]string) error {
	for field, value := range fields {
		if err := s.Scan(field, value); err != nil {
			return err
		}
	}
	return nil
}

func (s Scanner) ScanValidated(req domain.ValidatedRequest) error {
	return s.ScanFields(map[string]string{
		"expected":        req.Expected,
		"observed":        req.Observed,
		"impact":          req.Impact,
		"locus":           req.Locus,
		"severity":        req.Severity,
		"scope":           req.Scope,
		"suggestion":      req.Suggestion,
		"idempotency_key": req.IdempotencyKey,
	})
}

func (s Scanner) Scan(field string, value string) error {
	if err := scanCredentialAssignments(field, value); err != nil {
		return err
	}
	if err := scanCredentialURLs(field, value); err != nil {
		return err
	}
	for _, rule := range s.rules {
		if rule.re.MatchString(value) {
			return Denial{Field: field, Rule: rule.name}
		}
	}
	return nil
}

func (s Scanner) ScanPersistable(field string, body []byte) error {
	value := string(body)
	if err := scanCredentialAssignments(field, value); err != nil {
		return err
	}
	if err := scanCredentialURLs(field, value); err != nil {
		return err
	}
	for _, rule := range s.rules {
		if rule.re.Match(body) {
			return Denial{Field: field, Rule: rule.name}
		}
	}
	return nil
}

func IsDenied(err error) bool {
	var denial Denial
	return errors.As(err, &denial)
}

var urlCandidate = regexp.MustCompile(`[A-Za-z][A-Za-z0-9+.-]*://[^\s<>"']+`)
var assignmentCandidate = regexp.MustCompile(`(?i)(?:^|[^A-Za-z0-9_])["']?([A-Za-z][A-Za-z0-9_.\-\[\]]{1,64})["']?\s*[:=]`)
var escapedAssignmentCandidate = regexp.MustCompile(`(?i)(?:^|[^A-Za-z0-9_])\\["']([A-Za-z][A-Za-z0-9_.\-\[\]]{1,64})\\["']\s*[:=]`)

func scanCredentialAssignments(field string, value string) error {
	for _, re := range []*regexp.Regexp{assignmentCandidate, escapedAssignmentCandidate} {
		for _, match := range re.FindAllStringSubmatch(value, -1) {
			if len(match) < 2 {
				continue
			}
			if sensitiveKey(match[1]) {
				return Denial{Field: field, Rule: "credential-assignment"}
			}
		}
	}
	return nil
}

func scanCredentialURLs(field string, value string) error {
	for _, raw := range urlCandidate.FindAllString(value, -1) {
		candidate := strings.TrimRight(raw, ".,;)]}")
		parsed, err := url.Parse(candidate)
		if err != nil {
			return Denial{Field: field, Rule: "url-malformed"}
		}
		if parsed.Scheme == "" || parsed.Host == "" {
			continue
		}
		if parsed.User != nil {
			return Denial{Field: field, Rule: "url-userinfo-credential"}
		}
		query, err := url.ParseQuery(parsed.RawQuery)
		if err != nil {
			return Denial{Field: field, Rule: "url-query-malformed"}
		}
		for key, values := range query {
			if len(values) == 0 {
				continue
			}
			if !sensitiveKey(key) {
				continue
			}
			for _, value := range values {
				if value != "" {
					return Denial{Field: field, Rule: "url-query-credential"}
				}
			}
		}
	}
	return nil
}

func sensitiveKey(key string) bool {
	for _, part := range keyParts(key) {
		if sensitiveNormalizedKey(normalizeKey(part)) {
			return true
		}
	}
	return false
}

func sensitiveNormalizedKey(key string) bool {
	switch key {
	case "access_token", "auth", "authorization", "auth_token", "id_token", "refresh_token", "token",
		"api_key", "apikey", "key", "client_secret", "secret",
		"password", "passwd", "pwd",
		"session", "sessionid", "session_id", "session_token",
		"sig", "signature", "sharedaccesssignature", "awsaccesskeyid",
		"x_amz_signature", "x_amz_credential", "x_amz_security_token",
		"x_goog_signature", "x_goog_credential", "x_goog_algorithm":
		return true
	default:
		return false
	}
}

func normalizeKey(key string) string {
	key = strings.ReplaceAll(strings.ReplaceAll(key, "-", "_"), ".", "_")
	var out []rune
	var prev rune
	for i, r := range key {
		if i > 0 && r >= 'A' && r <= 'Z' && ((prev >= 'a' && prev <= 'z') || (prev >= '0' && prev <= '9')) {
			out = append(out, '_')
		}
		out = append(out, r)
		prev = r
	}
	return strings.ToLower(string(out))
}

func keyParts(key string) []string {
	var parts []string
	var current strings.Builder
	for _, r := range key {
		switch r {
		case '[', ']', '.':
			if current.Len() > 0 {
				parts = append(parts, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(r)
		}
	}
	if current.Len() > 0 {
		parts = append(parts, current.String())
	}
	if len(parts) == 0 {
		return []string{key}
	}
	parts = append(parts, key)
	return parts
}
