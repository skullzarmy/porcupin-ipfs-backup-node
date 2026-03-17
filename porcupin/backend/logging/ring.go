package logging

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
)

// Entry is a single log record stored in the ring buffer.
type Entry struct {
	Time    time.Time `json:"time"`
	Level   string    `json:"level"`   // "DEBUG", "INFO", "WARN", "ERROR"
	Message string    `json:"message"`
}

// RingHandler is a slog.Handler that stores log entries in a fixed-size circular buffer.
type RingHandler struct {
	mu       sync.RWMutex
	entries  []Entry
	capacity int
	pos      int
	count    int
	minLevel slog.Level
}

// NewRingHandler creates a RingHandler with the given capacity and minimum log level.
func NewRingHandler(capacity int, minLevel slog.Level) *RingHandler {
	return &RingHandler{
		entries:  make([]Entry, capacity),
		capacity: capacity,
		minLevel: minLevel,
	}
}

// Enabled implements slog.Handler.
func (h *RingHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.minLevel
}

// Handle implements slog.Handler. Stores the record in the ring buffer,
// including any structured attributes appended to the message.
func (h *RingHandler) Handle(_ context.Context, r slog.Record) error {
	var extra string
	r.Attrs(func(a slog.Attr) bool {
		extra += " " + a.Key + "=" + fmt.Sprintf("%v", a.Value.Any())
		return true
	})
	h.store(r, extra)
	return nil
}

// store writes a record to the ring buffer with a pre-built attribute suffix.
func (h *RingHandler) store(r slog.Record, extra string) {
	msg := r.Message
	if extra != "" {
		msg += extra
	}
	entry := Entry{
		Time:    r.Time,
		Level:   levelString(r.Level),
		Message: msg,
	}
	h.mu.Lock()
	h.entries[h.pos] = entry
	h.pos = (h.pos + 1) % h.capacity
	if h.count < h.capacity {
		h.count++
	}
	h.mu.Unlock()
}

// WithAttrs implements slog.Handler. Returns a wrapper that prepends attrs to each message.
func (h *RingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return newPrefixedHandler(h, attrs, "")
}

// WithGroup implements slog.Handler. Returns a wrapper that namespaces subsequent attrs.
func (h *RingHandler) WithGroup(name string) slog.Handler {
	return &prefixedRingHandler{ring: h, attrPrefix: "", groupPrefix: name + "."}
}

// prefixedRingHandler wraps a RingHandler with pre-computed attribute context for
// structured logging calls made via WithAttrs or WithGroup.
type prefixedRingHandler struct {
	ring        *RingHandler
	attrPrefix  string // pre-formatted " key=val key=val"
	groupPrefix string // e.g. "db." for WithGroup("db")
}

func newPrefixedHandler(ring *RingHandler, attrs []slog.Attr, groupPrefix string) *prefixedRingHandler {
	prefix := ""
	for _, a := range attrs {
		prefix += " " + groupPrefix + a.Key + "=" + fmt.Sprintf("%v", a.Value.Any())
	}
	return &prefixedRingHandler{ring: ring, attrPrefix: prefix, groupPrefix: groupPrefix}
}

func (h *prefixedRingHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.ring.Enabled(ctx, level)
}

func (h *prefixedRingHandler) Handle(_ context.Context, r slog.Record) error {
	extra := h.attrPrefix
	r.Attrs(func(a slog.Attr) bool {
		extra += " " + h.groupPrefix + a.Key + "=" + fmt.Sprintf("%v", a.Value.Any())
		return true
	})
	h.ring.store(r, extra)
	return nil
}

func (h *prefixedRingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	prefix := h.attrPrefix
	for _, a := range attrs {
		prefix += " " + h.groupPrefix + a.Key + "=" + fmt.Sprintf("%v", a.Value.Any())
	}
	return &prefixedRingHandler{ring: h.ring, attrPrefix: prefix, groupPrefix: h.groupPrefix}
}

func (h *prefixedRingHandler) WithGroup(name string) slog.Handler {
	return &prefixedRingHandler{
		ring:        h.ring,
		attrPrefix:  h.attrPrefix,
		groupPrefix: h.groupPrefix + name + ".",
	}
}

// Entries returns up to limit recent entries, optionally filtered by minimum level string.
// minLevel: "DEBUG", "INFO", "WARN", "ERROR", or "" for all.
func (h *RingHandler) Entries(limit int, minLevel string) []Entry {
	min := parseLevelString(minLevel)

	h.mu.RLock()
	defer h.mu.RUnlock()

	// Reconstruct ordered slice from circular buffer
	ordered := make([]Entry, 0, h.count)
	start := 0
	if h.count == h.capacity {
		start = h.pos // oldest entry
	}
	for i := 0; i < h.count; i++ {
		e := h.entries[(start+i)%h.capacity]
		if parseLevelString(e.Level) >= min {
			ordered = append(ordered, e)
		}
	}

	// Return the most recent 'limit' entries
	if limit > 0 && len(ordered) > limit {
		ordered = ordered[len(ordered)-limit:]
	}
	return ordered
}

// ExportText returns all buffered entries as newline-delimited formatted text.
func (h *RingHandler) ExportText() string {
	entries := h.Entries(0, "")
	var sb strings.Builder
	for _, e := range entries {
		fmt.Fprintf(&sb, "%s [%s] %s\n",
			e.Time.Format("2006-01-02T15:04:05.000Z07:00"),
			e.Level,
			e.Message,
		)
	}
	return sb.String()
}

func levelString(l slog.Level) string {
	switch {
	case l >= slog.LevelError:
		return "ERROR"
	case l >= slog.LevelWarn:
		return "WARN"
	case l >= slog.LevelInfo:
		return "INFO"
	default:
		return "DEBUG"
	}
}

func parseLevelString(s string) slog.Level {
	switch strings.ToUpper(s) {
	case "ERROR":
		return slog.LevelError
	case "WARN":
		return slog.LevelWarn
	case "INFO":
		return slog.LevelInfo
	case "DEBUG":
		return slog.LevelDebug
	default:
		// Empty string ("All" filter) and unrecognized values → show everything from DEBUG up.
		return slog.LevelDebug
	}
}
