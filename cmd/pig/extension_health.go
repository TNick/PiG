package main

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/MichaelKinsy/PiG/coding/extension"
	"github.com/MichaelKinsy/PiG/coding/extension/host/subprocess"
	"github.com/MichaelKinsy/PiG/internal/codingagent"
)

const extensionHealthCommandName = "pig-extensions"

// extensionHealthExtension adds a native command that reports subprocess host
// state without contacting extension processes.
// pig additive (D103): product-neutral host diagnostics remain in Stock PiG.
func extensionHealthExtension(host *subprocess.Host) extension.Extension {
	const path = "<inline:pig-extension-health>"
	info := codingagent.PiSourceInfo{
		Path: path, Source: "inline", Scope: "temporary", Origin: "top-level",
	}
	return extension.Extension{
		Name:         "pig-extension-health",
		Path:         path,
		ResolvedPath: path,
		SourceInfo:   info,
		Commands: map[string]extension.RegisteredCommand{
			extensionHealthCommandName: {
				Name:        extensionHealthCommandName,
				SourceInfo:  info,
				Description: "Show subprocess extension health and outstanding requests",
				Handler: func(ctx context.Context, _ string) error {
					var snapshot subprocess.HostHealth
					if host != nil {
						snapshot = host.HealthSnapshot()
					}
					commandContext := extension.FromContext(ctx)
					if commandContext == nil {
						return fmt.Errorf("extension health command has no extension context")
					}
					ui, err := commandContext.UI()
					if err != nil {
						return err
					}
					ui.Notify(formatExtensionHealth(snapshot), "info")
					return nil
				},
			},
		},
		CommandOrder: []string{extensionHealthCommandName},
	}
}

func formatExtensionHealth(snapshot subprocess.HostHealth) string {
	var output strings.Builder
	output.WriteString("Subprocess extensions:")
	if len(snapshot.Extensions) == 0 {
		output.WriteString("\n  none loaded")
	}
	for _, item := range snapshot.Extensions {
		fmt.Fprintf(&output, "\n  %d. %s — %s (%s)", item.RegistrationOrder, item.Name, item.State, item.Path)
		for _, request := range item.Connection.Outstanding {
			fmt.Fprintf(&output, "\n     request %s: %s outstanding for %s", request.ID, request.Operation, request.Elapsed.Round(time.Second))
		}
		if item.Connection.Failure != "" {
			fmt.Fprintf(&output, "\n     connection failure: %s", item.Connection.Failure)
		}
		if item.LastFailure != "" {
			fmt.Fprintf(&output, "\n     last failure: %s", item.LastFailure)
		}
	}
	if len(snapshot.QuarantinedCells) > 0 {
		keys := slices.Sorted(func(yield func(string) bool) {
			for key := range snapshot.QuarantinedCells {
				if !yield(key) {
					return
				}
			}
		})
		output.WriteString("\nQuarantined packed cells:")
		for _, key := range keys {
			fmt.Fprintf(&output, "\n  %s: %s", key, snapshot.QuarantinedCells[key])
		}
	}
	return output.String()
}
