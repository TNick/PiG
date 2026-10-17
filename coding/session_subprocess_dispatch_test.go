package coding

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/coding/extension/host/subprocess"
)

func TestStalledSubprocessEventDoesNotBlockNativeBash(t *testing.T) {
	root := t.TempDir()
	entrypoint := filepath.Join(root, "hung.mjs")
	source := `export default function(pi) {
  pi.on("tool_call", () => { while (true) {} });
}
`
	if err := os.WriteFile(entrypoint, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}

	host := subprocess.NewHostWithConfigRoot(root, filepath.Join(root, "config"))
	t.Cleanup(func() { host.Shutdown("test") })
	loaded, loadErrors := host.LoadAll(t.Context(), []subprocess.ExtConfig{{
		Name: "hung", Source: entrypoint, Enabled: true,
	}})
	if len(loadErrors) != 0 || len(loaded) != 1 {
		t.Fatalf("loaded extensions = %d, errors = %v", len(loaded), loadErrors)
	}

	marker := filepath.Join(root, "bash-ran")
	bashCall := func([]ai.Message) *ai.AssistantMessage {
		return &ai.AssistantMessage{
			Content: []ai.AssistantContentBlock{ai.ToolCall{
				ID: "call-bash", Name: "bash",
				Arguments: ai.JsonObject{"command": "touch " + marker, "timeout": float64(15)},
			}},
			Provider: "faux", Model: "faux-1", StopReason: ai.StopReasonToolUse,
			Timestamp: time.Now().UnixMilli(),
		}
	}
	services := newTestServices(t)
	runtime, err := NewRuntime(RuntimeOptions{Services: services, NewExtensions: loaded})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close() })
	provider := &scriptedProvider{responses: []scriptedResponse{
		bashCall,
		fauxReply("done", ai.StopReasonStop, 0),
	}}
	model := &ai.Model{
		ID: "faux-1", DisplayName: "faux-1", Provider: provider,
		Capabilities: ai.ModelCapabilities{ContextWindow: 128_000},
	}
	session, err := runtime.New(SessionStartOptions{Model: model, NoSession: true})
	if err != nil {
		t.Fatal(err)
	}
	drainSessionEvents(t, session)

	messages, err := session.Send(context.Background(), "run bash")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("native bash did not execute after event handler quarantine: %v", err)
	}
	results := toolResultMessages(messages)
	if len(results) != 1 || results[0].IsError {
		t.Fatalf("bash results = %+v, want one successful result", results)
	}
	if got := host.Extensions(); len(got) != 0 {
		t.Fatalf("quarantined extension list = %d entries, want none", len(got))
	}
	health := host.HealthSnapshot()
	if len(health.Extensions) != 1 || health.Extensions[0].Name != "hung" {
		t.Fatalf("extension health = %+v, want the quarantined extension", health.Extensions)
	}
	if !strings.Contains(health.Extensions[0].LastFailure, "handler_stalled") {
		t.Fatalf("extension health failure = %q, want the stall reason", health.Extensions[0].LastFailure)
	}
}
