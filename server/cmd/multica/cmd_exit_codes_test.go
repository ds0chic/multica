package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

// `runtime update --wait` reports a failed or timed-out update as an error so
// the process exits non-zero; scripts chain on it with &&.
func TestRuntimeUpdateWaitFailsWhenUpdateFails(t *testing.T) {
	redirectTestHome(t, t.TempDir())
	t.Setenv("MULTICA_TOKEN", "test-token")
	t.Setenv("MULTICA_WORKSPACE_ID", "ws-123")

	for _, status := range []string{"failed", "timeout"} {
		t.Run(status, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				st := "pending"
				if r.Method == http.MethodGet {
					st = status
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"id": "u-1", "status": st, "error": "boom"})
			}))
			defer srv.Close()
			t.Setenv("MULTICA_SERVER_URL", srv.URL)

			cmd := &cobra.Command{}
			cmd.Flags().String("target-version", "v9.9.9", "")
			cmd.Flags().Bool("wait", true, "")
			cmd.Flags().String("output", "table", "")

			done := make(chan error, 1)
			go func() { done <- runRuntimeUpdate(cmd, []string{"rt-1"}) }()
			select {
			case err := <-done:
				if err == nil || !strings.Contains(err.Error(), status) {
					t.Fatalf("runRuntimeUpdate error = %v, want an error mentioning %q", err, status)
				}
			case <-time.After(10 * time.Second):
				t.Fatal("runRuntimeUpdate did not return")
			}
		})
	}
}

// `setup self-host` against an unreachable server leaves the config alone and
// must also fail, rather than reporting success to a calling script.
func TestSetupSelfHostFailsWhenServerUnreachable(t *testing.T) {
	redirectTestHome(t, t.TempDir())
	t.Setenv("MULTICA_SERVER_URL", "http://127.0.0.1:1")
	t.Setenv("MULTICA_APP_URL", "http://127.0.0.1:3000")

	err := runSetupSelfHost(setupSelfHostCmd, nil)
	if err == nil || !strings.Contains(err.Error(), "not reachable") {
		t.Fatalf("runSetupSelfHost error = %v, want a not-reachable error", err)
	}
}
