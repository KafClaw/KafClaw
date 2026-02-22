package middleware

import (
	"regexp"
	"strings"

	"github.com/KafClaw/KafClaw/internal/config"
)

// DetectorMatch represents a single detection hit.
type DetectorMatch struct {
	Type  string // e.g. "email", "ssn", "api_key"
	Value string // the matched text
	Start int    // byte offset in source string
	End   int    // byte offset end
}

// Detector scans text for sensitive patterns.
type Detector struct {
	piiDetectors    []namedRegex
	secretDetectors []namedRegex
	customDetectors []namedRegex
}

type namedRegex struct {
	name string
	re   *regexp.Regexp
}

// Built-in PII patterns.
var builtinPII = map[string]string{
	"email":       `\b[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}\b`,
	"phone":       `(?:\+\d{1,3}[\s\-]?)?\(?\d{2,4}\)?[\s\-]?\d{3,4}[\s\-]?\d{3,4}\b`,
	"ssn":         `\b\d{3}-\d{2}-\d{4}\b`,
	"credit_card": `\b(?:\d{4}[\s\-]?){3}\d{4}\b`,
	"ip_address":  `\b(?:\d{1,3}\.){3}\d{1,3}\b`,
}

// Built-in secret patterns.
var builtinSecrets = map[string]string{
	"api_key":          `\b(?:sk-[A-Za-z0-9]{20,}|pk_[A-Za-z0-9]{20,}|AKIA[A-Z0-9]{16}|ghp_[A-Za-z0-9]{36}|gho_[A-Za-z0-9]{36}|glpat-[A-Za-z0-9\-]{20,})\b`,
	"bearer_token":     `Bearer\s+[A-Za-z0-9\-._~+/]+=*`,
	"private_key":      `-----BEGIN\s+[A-Z\s]*PRIVATE\s+KEY-----`,
	"password_literal": `(?i)(?:password|passwd|pwd)\s*[:=]\s*\S+`,
}

// NewDetector creates a detector from config settings.
func NewDetector(piiTypes, secretTypes []string, customPatterns []config.NamedPattern) *Detector {
	d := &Detector{}

	for _, pt := range piiTypes {
		pattern, ok := builtinPII[pt]
		if !ok {
			continue
		}
		re, err := regexp.Compile(pattern)
		if err != nil {
			continue
		}
		d.piiDetectors = append(d.piiDetectors, namedRegex{name: pt, re: re})
	}

	for _, st := range secretTypes {
		pattern, ok := builtinSecrets[st]
		if !ok {
			continue
		}
		re, err := regexp.Compile(pattern)
		if err != nil {
			continue
		}
		d.secretDetectors = append(d.secretDetectors, namedRegex{name: st, re: re})
	}

	for _, cp := range customPatterns {
		re, err := regexp.Compile(cp.Pattern)
		if err != nil {
			continue
		}
		d.customDetectors = append(d.customDetectors, namedRegex{name: cp.Name, re: re})
	}

	return d
}

// NewDefaultDetector creates a detector with all built-in PII and secret patterns.
func NewDefaultDetector() *Detector {
	piiTypes := make([]string, 0, len(builtinPII))
	for k := range builtinPII {
		piiTypes = append(piiTypes, k)
	}
	secretTypes := make([]string, 0, len(builtinSecrets))
	for k := range builtinSecrets {
		secretTypes = append(secretTypes, k)
	}
	return NewDetector(piiTypes, secretTypes, nil)
}

// Scan returns all matches found in the text.
func (d *Detector) Scan(text string) []DetectorMatch {
	var matches []DetectorMatch
	for _, nr := range d.piiDetectors {
		matches = append(matches, findMatches(nr, text)...)
	}
	for _, nr := range d.secretDetectors {
		matches = append(matches, findMatches(nr, text)...)
	}
	for _, nr := range d.customDetectors {
		matches = append(matches, findMatches(nr, text)...)
	}
	return matches
}

// ScanPII returns only PII matches.
func (d *Detector) ScanPII(text string) []DetectorMatch {
	var matches []DetectorMatch
	for _, nr := range d.piiDetectors {
		matches = append(matches, findMatches(nr, text)...)
	}
	return matches
}

// ScanSecrets returns only secret matches.
func (d *Detector) ScanSecrets(text string) []DetectorMatch {
	var matches []DetectorMatch
	for _, nr := range d.secretDetectors {
		matches = append(matches, findMatches(nr, text)...)
	}
	return matches
}

// HasMatches returns true if any pattern matches the text.
func (d *Detector) HasMatches(text string) bool {
	for _, nr := range d.piiDetectors {
		if nr.re.MatchString(text) {
			return true
		}
	}
	for _, nr := range d.secretDetectors {
		if nr.re.MatchString(text) {
			return true
		}
	}
	for _, nr := range d.customDetectors {
		if nr.re.MatchString(text) {
			return true
		}
	}
	return false
}

// Redact replaces all detected matches in the text with [REDACTED:<type>].
func (d *Detector) Redact(text string) string {
	// Process all detectors
	allDetectors := make([]namedRegex, 0, len(d.piiDetectors)+len(d.secretDetectors)+len(d.customDetectors))
	allDetectors = append(allDetectors, d.piiDetectors...)
	allDetectors = append(allDetectors, d.secretDetectors...)
	allDetectors = append(allDetectors, d.customDetectors...)

	result := text
	for _, nr := range allDetectors {
		replacement := "[REDACTED:" + strings.ToUpper(nr.name) + "]"
		result = nr.re.ReplaceAllString(result, replacement)
	}
	return result
}

func findMatches(nr namedRegex, text string) []DetectorMatch {
	locs := nr.re.FindAllStringIndex(text, -1)
	matches := make([]DetectorMatch, 0, len(locs))
	for _, loc := range locs {
		matches = append(matches, DetectorMatch{
			Type:  nr.name,
			Value: text[loc[0]:loc[1]],
			Start: loc[0],
			End:   loc[1],
		})
	}
	return matches
}

// ContainsKeywords checks if text contains any of the given keywords (case-insensitive).
func ContainsKeywords(text string, keywords []string) []string {
	lower := strings.ToLower(text)
	var found []string
	for _, kw := range keywords {
		if strings.Contains(lower, strings.ToLower(kw)) {
			found = append(found, kw)
		}
	}
	return found
}
