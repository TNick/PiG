package subprocess

import (
	"testing"
	"time"

	"github.com/MichaelKinsy/PiG/coding/extension"
)

func TestHostHealthSnapshotReportsStalledExtensionsWithoutCallingThem(t *testing.T) {
	started := time.Now().Add(-3 * time.Second)
	conn := newConnWithOptions("slow", nil, connOptions{})
	conn.pendingInfo["request-1"] = pendingRequestInfo{
		started: started, operation: "tool_call",
	}
	managed := &managedExt{
		config: ExtConfig{Name: "slow", selectedPath: "/extensions/slow.ts"},
		ext:    &extension.Extension{Name: "slow", Path: "/extensions/slow.ts"},
	}
	managed.conn.Store(conn)
	host := &Host{
		exts:               map[string]*managedExt{"slow": managed},
		loadOrder:          map[string]int{"slow": 9},
		configSelectedPath: map[string]string{"slow": "/configured/slow.ts"},
		extensionFailures:  map[string]string{"slow": "handler_stalled: test"},
		quarantinedCells:   map[string]string{"cell-1": "heartbeat timeout"},
	}

	health := host.HealthSnapshot()
	if len(health.Extensions) != 1 {
		t.Fatalf("extension count = %d, want 1", len(health.Extensions))
	}
	got := health.Extensions[0]
	if got.Name != "slow" || got.Path != "/extensions/slow.ts" || got.RegistrationOrder != 10 {
		t.Fatalf("extension identity = %+v", got)
	}
	if got.State != "connected" || got.LastFailure != "handler_stalled: test" {
		t.Fatalf("extension state = %+v", got)
	}
	if len(got.Connection.Outstanding) != 1 {
		t.Fatalf("outstanding requests = %+v, want one", got.Connection.Outstanding)
	}
	request := got.Connection.Outstanding[0]
	if request.ID != "request-1" || request.Operation != "tool_call" || request.Elapsed < 3*time.Second {
		t.Fatalf("outstanding request = %+v", request)
	}
	if health.QuarantinedCells["cell-1"] != "heartbeat timeout" {
		t.Fatalf("quarantined cells = %v", health.QuarantinedCells)
	}
}
