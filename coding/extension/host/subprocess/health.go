package subprocess

import (
	"slices"
	"time"
)

// RequestHealth describes one extension request that has not answered yet.
type RequestHealth struct {
	ID        string
	Operation string
	Elapsed   time.Duration
}

// ConnectionHealth is a read-only snapshot of one extension connection.
type ConnectionHealth struct {
	Connected   bool
	Failure     string
	Outstanding []RequestHealth
}

// ExtensionHealth describes one extension in the subprocess host.
type ExtensionHealth struct {
	Name              string
	Path              string
	RegistrationOrder int
	State             string
	LastFailure       string
	Connection        ConnectionHealth
}

// HostHealth is a point-in-time snapshot of loaded extensions and quarantined
// packed cells.
type HostHealth struct {
	Extensions       []ExtensionHealth
	QuarantinedCells map[string]string
}

// HealthSnapshot reports pending requests and transport state without sending
// a request to the extension process.
// pig additive (D103): expose subprocess liveness as a local snapshot.
func (c *Conn) HealthSnapshot() ConnectionHealth {
	c.pendingMu.Lock()
	outstanding := make([]RequestHealth, 0, len(c.pendingInfo))
	now := c.clock.Now()
	for id, info := range c.pendingInfo {
		outstanding = append(outstanding, RequestHealth{
			ID: id, Operation: info.operation,
			Elapsed: max(0, now.Sub(info.started)),
		})
	}
	c.pendingMu.Unlock()
	slices.SortFunc(outstanding, func(a, b RequestHealth) int {
		if a.Elapsed > b.Elapsed {
			return -1
		}
		if a.Elapsed < b.Elapsed {
			return 1
		}
		return 0
	})

	failure := c.failureError()
	connected := !c.closed.Load() && !c.closing.Load() && failure == nil
	result := ConnectionHealth{
		Connected:   connected,
		Outstanding: outstanding,
	}
	if failure != nil {
		result.Failure = failure.Error()
	}
	return result
}

// HealthSnapshot returns host state using in-memory data only. It never
// contacts an extension and remains available when one extension is stalled.
// pig additive (D103): expose host health without dispatching through extensions.
func (h *Host) HealthSnapshot() HostHealth {
	h.mu.Lock()
	result := HostHealth{
		Extensions:       make([]ExtensionHealth, 0, len(h.exts)),
		QuarantinedCells: make(map[string]string, len(h.quarantinedCells)),
	}
	for key, reason := range h.quarantinedCells {
		result.QuarantinedCells[key] = reason
	}
	type extensionRecord struct {
		conn   *Conn
		status ExtensionHealth
	}
	records := make([]extensionRecord, 0, len(h.exts))
	active := make(map[string]struct{}, len(h.exts))
	for name, managed := range h.exts {
		active[name] = struct{}{}
		path := h.configSelectedPath[name]
		if path == "" {
			path = extConfigOrigin(managed.config)
		}
		order, hasOrder := h.loadOrder[name]
		if hasOrder {
			order++
		}
		status := ExtensionHealth{
			Name: name, Path: path, RegistrationOrder: order,
			State:       "starting",
			LastFailure: h.extensionFailures[name],
		}
		if managed.ext != nil {
			status.Name = managed.ext.Name
			if managed.ext.Path != "" {
				status.Path = managed.ext.Path
			}
		}
		records = append(records, extensionRecord{
			conn: managed.connection(), status: status,
		})
	}
	for name, failure := range h.extensionFailures {
		if _, exists := active[name]; exists {
			continue
		}
		order, hasOrder := h.loadOrder[name]
		if hasOrder {
			order++
		}
		records = append(records, extensionRecord{status: ExtensionHealth{
			Name: name, Path: h.configSelectedPath[name],
			RegistrationOrder: order, State: "quarantined",
			LastFailure: failure,
		}})
	}
	h.mu.Unlock()
	for _, record := range records {
		status := record.status
		if conn := record.conn; conn != nil {
			status.Connection = conn.HealthSnapshot()
			status.State = "connected"
			if !status.Connection.Connected {
				status.State = "disconnected"
			}
		}
		result.Extensions = append(result.Extensions, status)
	}
	slices.SortFunc(result.Extensions, func(a, b ExtensionHealth) int {
		if a.RegistrationOrder != b.RegistrationOrder {
			return a.RegistrationOrder - b.RegistrationOrder
		}
		if a.Name < b.Name {
			return -1
		}
		if a.Name > b.Name {
			return 1
		}
		return 0
	})
	return result
}
