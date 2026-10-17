package subprocess

import (
	"net"
	"strings"
	"testing"
)

func TestExtensionConnectionRetainsConfiguredIdentity(t *testing.T) {
	host := NewHostWithConfigRoot(t.TempDir(), t.TempDir())
	configs := []ExtConfig{
		{Name: "first", Enabled: true},
		{Name: "second", Enabled: true},
		{Name: "hooks", Source: "/ext/claude-clone-hooks/index.ts", Enabled: true},
	}
	host.recordLoadOrder(configs, true)

	local, peer := net.Pipe()
	t.Cleanup(func() { _ = local.Close(); _ = peer.Close() })
	me := &managedExt{config: configs[2], host: host}
	conn := host.newExtensionConn(me, local)
	failure := conn.unresponsiveFailure()
	if failure.Extension != "hooks" || failure.ExtensionPath != configs[2].Source || failure.RegistrationOrder != 3 {
		t.Fatalf("failure identity = %+v, want hooks at configured order 3", failure)
	}
	message := failure.Error()
	if !strings.Contains(message, `path="/ext/claude-clone-hooks/index.ts"`) || !strings.Contains(message, "registration_order=3") {
		t.Fatalf("failure message = %q, want extension path and registration order", message)
	}
}
