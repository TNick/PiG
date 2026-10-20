package cli

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/MichaelKinsy/PiG/coding/extension"
	"github.com/MichaelKinsy/PiG/coding/extension/host/subprocess"
)

type healthNotifyUI struct {
	extension.UIContext
	message string
}

func (ui *healthNotifyUI) Notify(message, _ string) {
	ui.message = message
}

func TestPigExtensionsCommandReportsHostHealth(t *testing.T) {
	ui := &healthNotifyUI{UIContext: extension.NoopUIContext}
	ctx := extension.WithContext(context.Background(), extension.NewContext(
		"", ui, func() error { return nil }, extension.ContextActions{},
	))
	command := extensionHealthExtension(nil).Commands[extensionHealthCommandName]
	if err := command.Handler(ctx, ""); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ui.message, "Subprocess extensions:\n  none loaded") {
		t.Fatalf("health notification = %q", ui.message)
	}
}

func TestFormatExtensionHealthIncludesIdentityRequestsAndQuarantine(t *testing.T) {
	message := formatExtensionHealth(subprocess.HostHealth{
		Extensions: []subprocess.ExtensionHealth{{
			Name: "semantic-code", Path: "/extensions/semantic-code.ts",
			RegistrationOrder: 11, State: "disconnected",
			LastFailure: "handler_stalled: tool_call",
			Connection: subprocess.ConnectionHealth{
				Failure: "extension_transport: closed",
				Outstanding: []subprocess.RequestHealth{{
					ID: "r42", Operation: "tool_call", Elapsed: 7 * time.Second,
				}},
			},
		}},
		QuarantinedCells: map[string]string{"cell-abc": "heartbeat timeout"},
	})
	for _, want := range []string{
		"11. semantic-code — disconnected (/extensions/semantic-code.ts)",
		"request r42: tool_call outstanding for 7s",
		"connection failure: extension_transport: closed",
		"last failure: handler_stalled: tool_call",
		"cell-abc: heartbeat timeout",
	} {
		if !strings.Contains(message, want) {
			t.Errorf("health report %q does not contain %q", message, want)
		}
	}
}
