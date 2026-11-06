package steps

import (
	"strings"
	"testing"
)

func TestScanCredentialsPositives(t *testing.T) {
	cases := []struct {
		name    string
		env     []string
		labels  map[string]string
		history []string
	}{
		{name: "name TOKEN", env: []string{"GITHUB_TOKEN=x"}},
		{name: "name lowercase secret", env: []string{"my_secret=x"}},
		{name: "name PASSWORD", env: []string{"DB_PASSWORD=x"}},
		{name: "name PASSWD", env: []string{"PASSWD=x"}},
		{name: "name API_KEY", env: []string{"OPENAI_API_KEY=x"}},
		{name: "name APIKEY", env: []string{"APIKEY=x"}},
		{name: "name CREDENTIAL", env: []string{"AWS_CREDENTIALS=x"}},
		{name: "name PRIVATE_KEY", env: []string{"SSH_PRIVATE_KEY=x"}},
		{name: "env ghp", env: []string{"FOO=gh" + "p_abcdef"}},
		{name: "env sk-ant", env: []string{"FOO=sk-" + "ant-api03-abc"}},
		{name: "env AKIA", env: []string{"FOO=AK" + "IAABCDEFGHIJKLMNOP"}},
		{name: "label github_pat", labels: map[string]string{"k": "github_" + "pat_11AAA"}},
		{name: "label gho", labels: map[string]string{"k": "gh" + "o_abc"}},
		{name: "history ghs", history: []string{"RUN echo gh" + "s_abc"}},
		{name: "history private key", history: []string{"RUN echo '-----BEGIN " + "RSA PRIVATE KEY-----'"}},
	}
	for _, c := range cases {
		if got := ScanCredentials(c.env, c.labels, c.history); got == "" {
			t.Errorf("%s: expected a finding", c.name)
		}
	}
}

func TestScanCredentialsNegatives(t *testing.T) {
	env := []string{
		"PATH=/opt/venv/bin:/usr/local/sbin:/usr/bin",
		"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1",
		"DEBIAN_FRONTEND=noninteractive",
	}
	labels := map[string]string{"org.opencontainers.image.version": "26.04"}
	history := []string{
		"RUN /bin/sh -c npm install -g @anthropic-ai/claude-code@2.0.0",
		"RUN echo AK" + "IA123 and -----BEGIN " + "CERTIFICATE-----",
	}
	if got := ScanCredentials(env, labels, history); got != "" {
		t.Errorf("clean image flagged: %s", got)
	}
	if got := ScanCredentials(nil, nil, nil); got != "" {
		t.Errorf("empty image flagged: %s", got)
	}
}

func TestScanCredentialsNeverPrintsValue(t *testing.T) {
	got := ScanCredentials([]string{"FOO=gh" + "p_SECRETVALUE"}, nil, nil)
	if got == "" || strings.Contains(got, "SECRETVALUE") {
		t.Errorf("finding must name the variable only: %q", got)
	}
	got = ScanCredentials(nil, nil, []string{"RUN echo gh" + "p_SECRETVALUE"})
	if got == "" || strings.Contains(got, "SECRETVALUE") {
		t.Errorf("finding must name the step only: %q", got)
	}
}
