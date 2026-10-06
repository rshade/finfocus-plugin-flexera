package main

import (
	"bytes"
	"strings"
	"testing"
)

func clearFlexeraEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"FLEXERA_CONFIG",
		"FLEXERA_ORG_ID",
		"FLEXERA_API_TOKEN",
		"FLEXERA_REFRESH_TOKEN",
		"FLEXERA_CLIENT_ID",
		"FLEXERA_CLIENT_SECRET",
		"FLEXERA_TLS_SKIP_VERIFY",
		"FLEXERA_LOG_LEVEL",
		"FLEXERA_BILLING_CENTER_IDS",
		"FLEXERA_REGION",
		"FLEXERA_BASE_URL",
	} {
		t.Setenv(key, "")
	}
}

func TestRunVersionDoesNotServe(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := run([]string{"--version"}, &stdout, &stderr); err != nil {
		t.Fatalf("run --version: %v", err)
	}
	if !strings.Contains(stdout.String(), "v0.1.0") {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}

	stdout.Reset()
	if err := run([]string{"--version-full"}, &stdout, &stderr); err != nil {
		t.Fatalf("run --version-full: %v", err)
	}
	if !strings.Contains(stdout.String(), "0.1.0") {
		t.Fatalf("full stdout = %q", stdout.String())
	}
}

func TestRunConfigFailureDoesNotServe(t *testing.T) {
	clearFlexeraEnv(t)
	var stdout, stderr bytes.Buffer
	err := run(nil, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "FLEXERA_ORG_ID") {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(stderr.String(), "listening") {
		t.Fatalf("stderr suggests serve started: %q", stderr.String())
	}
}

func TestRunAuthFailureWarnsAndRedacts(t *testing.T) {
	const secret = "refresh-token-super-secret-value"
	clearFlexeraEnv(t)
	t.Setenv("FLEXERA_ORG_ID", "not-numeric")
	t.Setenv("FLEXERA_REFRESH_TOKEN", secret)
	t.Setenv("FLEXERA_TLS_SKIP_VERIFY", "true")
	t.Setenv("FLEXERA_LOG_LEVEL", "warn")

	var stdout, stderr bytes.Buffer
	err := run(nil, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "numeric") {
		t.Fatalf("err = %v", err)
	}
	logged := stderr.String()
	if !strings.Contains(logged, "tlsSkipVerify") {
		t.Fatalf("stderr = %q", logged)
	}
	if strings.Contains(logged, secret) || strings.Contains(err.Error(), secret) {
		t.Fatal("refresh token appeared in logs or the error")
	}
	if strings.Contains(logged, "listening") {
		t.Fatalf("serve started: %q", logged)
	}

	stderr.Reset()
	t.Setenv("FLEXERA_LOG_LEVEL", "error")
	if err := run(nil, &stdout, &stderr); err == nil {
		t.Fatal("expected auth failure")
	}
	if strings.Contains(stderr.String(), "tlsSkipVerify") {
		t.Fatalf("warn logged at error level: %q", stderr.String())
	}
}
