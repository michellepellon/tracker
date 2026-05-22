// ABOUTME: Unit tests for the acai Reporter — exercises Pull/Push/Available with an injected fake command runner.
// ABOUTME: No real acai binary invoked; see integration_test.go for live-binary coverage.
package acai

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/2389-research/tracker/pkg/spec/reporter"
)

// fakeRunner captures the args it received and returns canned output.
type fakeRunner struct {
	stdout []byte
	stderr []byte
	err    error

	gotName string
	gotArgs []string
}

func (f *fakeRunner) run(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
	f.gotName = name
	f.gotArgs = args
	return f.stdout, f.stderr, f.err
}

func newReporterWithRunner(t *testing.T, fr *fakeRunner) *Reporter {
	t.Helper()
	return New(WithRunner(fr.run))
}

func target() reporter.Target {
	return reporter.Target{
		Feature:        "cognitoforms-py",
		Product:        "cognitoforms-py",
		Implementation: "main",
	}
}

// --- Available ---

func TestAvailable_TrueWhenCLISucceeds(t *testing.T) {
	fr := &fakeRunner{stdout: []byte(`{"feature":"cognitoforms-py","requirements":[]}`)}
	r := newReporterWithRunner(t, fr)
	if !r.Available(context.Background()) {
		t.Errorf("Available = false, want true when CLI exits 0")
	}
}

func TestAvailable_FalseWhenMissingToken(t *testing.T) {
	fr := &fakeRunner{
		stderr: []byte("Missing API bearer token configuration."),
		err:    errors.New("exit status 1"),
	}
	r := newReporterWithRunner(t, fr)
	if r.Available(context.Background()) {
		t.Errorf("Available = true, want false when CLI reports missing token")
	}
}

func TestAvailable_FalseWhenBinaryMissing(t *testing.T) {
	r := New(
		WithRunner(func(_ context.Context, _ string, _ ...string) ([]byte, []byte, error) {
			return nil, nil, &noBinaryError{}
		}),
		WithLookPath(func(_ string) (string, error) {
			return "", errors.New("executable file not found in $PATH")
		}),
	)
	if r.Available(context.Background()) {
		t.Errorf("Available = true, want false when binary missing")
	}
}

// --- Pull ---

func TestPull_ParsesAcaiJSON(t *testing.T) {
	fr := &fakeRunner{stdout: []byte(`{
		"feature": "cognitoforms-py",
		"requirements": [
			{"acid": "cognitoforms-py.AUTH.1", "status": "pass", "comment": "src/auth.py:42"},
			{"acid": "cognitoforms-py.AUTH.2", "status": "fail", "comment": "tests/test_auth.py:18: assertion"},
			{"acid": "cognitoforms-py.CLIENT.1", "status": "blocked", "comment": ""},
			{"acid": "cognitoforms-py.CLIENT.2", "status": "pending", "comment": ""},
			{"acid": "cognitoforms-py.CLIENT.3", "status": "", "comment": ""}
		]
	}`)}
	r := newReporterWithRunner(t, fr)

	got, err := r.Pull(context.Background(), target())
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if len(got) != 5 {
		t.Fatalf("got %d statuses, want 5", len(got))
	}
	cases := map[string]reporter.State{
		"cognitoforms-py.AUTH.1":   reporter.StatePass,
		"cognitoforms-py.AUTH.2":   reporter.StateFail,
		"cognitoforms-py.CLIENT.1": reporter.StateBlocked,
		"cognitoforms-py.CLIENT.2": reporter.StatePending,
		"cognitoforms-py.CLIENT.3": reporter.StatePending,
	}
	for acid, want := range cases {
		if got[acid].State != want {
			t.Errorf("%s state = %v, want %v", acid, got[acid].State, want)
		}
	}
	if got["cognitoforms-py.AUTH.1"].Comment != "src/auth.py:42" {
		t.Errorf("comment dropped: %#v", got["cognitoforms-py.AUTH.1"])
	}
}

func TestPull_InvokesCorrectArgv(t *testing.T) {
	fr := &fakeRunner{stdout: []byte(`{"feature":"cognitoforms-py","requirements":[]}`)}
	r := newReporterWithRunner(t, fr)

	_, _ = r.Pull(context.Background(), target())

	if fr.gotName != "acai" {
		t.Errorf("ran %q, want acai", fr.gotName)
	}
	want := []string{
		"feature", "cognitoforms-py",
		"--product", "cognitoforms-py",
		"--impl", "main",
		"--json",
		"--include-refs",
	}
	if !equalSlice(fr.gotArgs, want) {
		t.Errorf("argv = %v, want %v", fr.gotArgs, want)
	}
}

func TestPull_EmptyResultWhenUnavailable(t *testing.T) {
	r := New(WithLookPath(func(string) (string, error) {
		return "", errors.New("not found")
	}))
	got, err := r.Pull(context.Background(), target())
	if err != nil {
		t.Errorf("Pull on unavailable reporter returned err=%v, want nil", err)
	}
	if len(got) != 0 {
		t.Errorf("Pull on unavailable reporter returned %d entries, want 0", len(got))
	}
}

func TestPull_MalformedJSONReturnsError(t *testing.T) {
	fr := &fakeRunner{stdout: []byte("not json")}
	r := newReporterWithRunner(t, fr)
	_, err := r.Pull(context.Background(), target())
	if err == nil {
		t.Fatalf("expected error for malformed JSON")
	}
}

func TestPull_CLIErrorPassedThrough(t *testing.T) {
	fr := &fakeRunner{
		stderr: []byte("network unreachable"),
		err:    errors.New("exit 1"),
	}
	r := newReporterWithRunner(t, fr)
	_, err := r.Pull(context.Background(), target())
	if err == nil {
		t.Fatalf("expected error from failing CLI")
	}
	if !strings.Contains(err.Error(), "network unreachable") {
		t.Errorf("error %q should mention CLI stderr", err)
	}
}

// --- Push ---

func TestPush_InvokesCorrectArgv(t *testing.T) {
	fr := &fakeRunner{}
	r := newReporterWithRunner(t, fr)

	updates := []reporter.Status{
		{ACID: "cognitoforms-py.AUTH.1", State: reporter.StatePass, Comment: "src/auth.py:42"},
		{ACID: "cognitoforms-py.AUTH.2", State: reporter.StateFail, Comment: "fail!"},
	}
	if err := r.Push(context.Background(), target(), updates); err != nil {
		t.Fatalf("Push: %v", err)
	}

	if fr.gotName != "acai" || len(fr.gotArgs) < 2 || fr.gotArgs[0] != "set-status" {
		t.Fatalf("ran %q %v, want acai set-status ...", fr.gotName, fr.gotArgs)
	}
	jsonArg := fr.gotArgs[1]
	for _, want := range []string{
		`cognitoforms-py.AUTH.1`,
		`"status":"pass"`,
		`src/auth.py:42`,
		`cognitoforms-py.AUTH.2`,
		`"status":"fail"`,
	} {
		if !strings.Contains(jsonArg, want) {
			t.Errorf("JSON arg missing %q; got: %s", want, jsonArg)
		}
	}
	// product + impl flags must be present after the JSON.
	tail := strings.Join(fr.gotArgs[2:], " ")
	if !strings.Contains(tail, "--product cognitoforms-py") {
		t.Errorf("Push argv missing --product: %v", fr.gotArgs)
	}
	if !strings.Contains(tail, "--impl main") {
		t.Errorf("Push argv missing --impl: %v", fr.gotArgs)
	}
}

func TestPush_ErrUnavailableWhenBinaryMissing(t *testing.T) {
	r := New(WithLookPath(func(string) (string, error) {
		return "", errors.New("not found")
	}))
	err := r.Push(context.Background(), target(), []reporter.Status{
		{ACID: "x.Y.1", State: reporter.StatePass},
	})
	if !errors.Is(err, reporter.ErrUnavailable) {
		t.Errorf("Push on unavailable reporter returned %v, want ErrUnavailable", err)
	}
}

func TestPush_CLIErrorPassedThrough(t *testing.T) {
	fr := &fakeRunner{
		stderr: []byte("server down"),
		err:    errors.New("exit 1"),
	}
	r := newReporterWithRunner(t, fr)
	err := r.Push(context.Background(), target(), []reporter.Status{
		{ACID: "x.Y.1", State: reporter.StatePass},
	})
	if err == nil {
		t.Fatalf("expected error from failing CLI")
	}
	if !strings.Contains(err.Error(), "server down") {
		t.Errorf("error %q should mention CLI stderr", err)
	}
}

func TestPush_EmptyUpdatesIsNoOp(t *testing.T) {
	fr := &fakeRunner{}
	r := newReporterWithRunner(t, fr)
	if err := r.Push(context.Background(), target(), nil); err != nil {
		t.Errorf("Push with empty updates returned %v, want nil", err)
	}
	if fr.gotName != "" {
		t.Errorf("Push with empty updates invoked CLI %q %v, want no invocation", fr.gotName, fr.gotArgs)
	}
}

// --- parseState ---

func TestParseState(t *testing.T) {
	cases := map[string]reporter.State{
		"pass":    reporter.StatePass,
		"passed":  reporter.StatePass,
		"fail":    reporter.StateFail,
		"failed":  reporter.StateFail,
		"blocked": reporter.StateBlocked,
		"pending": reporter.StatePending,
		"":        reporter.StatePending,
		"weird":   reporter.StateUnknown,
		"PASS":    reporter.StatePass, // case-insensitive
	}
	for in, want := range cases {
		if got := parseState(in); got != want {
			t.Errorf("parseState(%q) = %v, want %v", in, got, want)
		}
	}
}

// --- Registration ---

func TestRegistered(t *testing.T) {
	r, ok := reporter.Lookup("acai")
	if !ok {
		t.Fatalf("acai reporter not registered")
	}
	if r.Name() != "acai" {
		t.Errorf("Name = %q, want acai", r.Name())
	}
}

// --- helpers ---

type noBinaryError struct{}

func (*noBinaryError) Error() string { return "binary not found" }

func equalSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
