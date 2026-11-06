// SSR-IMG-005: credential patterns checked on the built image's env, labels and history.
package steps

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// credentialNames are matched case-insensitively as substrings of an env var name.
var credentialNames = []string{"TOKEN", "SECRET", "PASSWORD", "PASSWD", "API_KEY", "APIKEY", "CREDENTIAL", "PRIVATE_KEY"}

var credentialValue = regexp.MustCompile(`ghp_|gho_|ghs_|github_pat_|sk-ant-|AKIA[0-9A-Z]{16}|-----BEGIN[^\n]*PRIVATE KEY`)

// CredentialName reports whether an env var or build-arg name looks like a credential name.
func CredentialName(name string) bool {
	up := strings.ToUpper(name)
	for _, c := range credentialNames {
		if strings.Contains(up, c) {
			return true
		}
	}
	return false
}

// CredentialValue reports whether v contains a known credential pattern.
func CredentialValue(v string) bool { return credentialValue.MatchString(v) }

// ScanCredentials returns a description of the first finding, or "" when clean.
// The description names the env var, label key or history entry, never the matched value.
func ScanCredentials(env []string, labels map[string]string, history []string) string {
	for _, e := range env {
		name, value, _ := strings.Cut(e, "=")
		if CredentialName(name) {
			return fmt.Sprintf("environment variable %s has a credential-like name", name)
		}
		if CredentialValue(value) {
			return fmt.Sprintf("environment variable %s has a credential-like value", name)
		}
	}
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if CredentialValue(labels[k]) {
			return fmt.Sprintf("label %s has a credential-like value", k)
		}
	}
	for i, h := range history {
		if CredentialValue(h) {
			return fmt.Sprintf("build history step %d (newest first, 1-based) has a credential-like value", i+1)
		}
	}
	return ""
}
