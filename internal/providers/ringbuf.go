package providers

import (
	"strings"
	"sync"
)

// RingBuffer is a thread-safe ring buffer that stores the last N lines of text.
// It implements io.Writer so it can be used as cmd.Stdout/Stderr.
type RingBuffer struct {
	mu       sync.Mutex
	lines    []string
	maxLines int
	partial  string // incomplete line (no newline yet)
}

// NewRingBuffer creates a ring buffer that stores up to maxLines lines.
func NewRingBuffer(maxLines int) *RingBuffer {
	if maxLines <= 0 {
		maxLines = 1000
	}
	return &RingBuffer{
		lines:    make([]string, 0, maxLines),
		maxLines: maxLines,
	}
}

// Write implements io.Writer. It splits input by newlines and stores complete lines.
func (rb *RingBuffer) Write(p []byte) (int, error) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	text := rb.partial + string(p)
	rb.partial = ""

	parts := strings.Split(text, "\n")

	// Last element is either empty (text ended with \n) or a partial line
	if len(parts) > 0 {
		rb.partial = parts[len(parts)-1]
		parts = parts[:len(parts)-1]
	}

	for _, line := range parts {
		rb.addLine(line)
	}

	return len(p), nil
}

func (rb *RingBuffer) addLine(line string) {
	if len(rb.lines) >= rb.maxLines {
		// Shift lines left
		copy(rb.lines, rb.lines[1:])
		rb.lines[len(rb.lines)-1] = line
	} else {
		rb.lines = append(rb.lines, line)
	}
}

// Lines returns the last n lines. If n <= 0 or n > stored, returns all stored lines.
func (rb *RingBuffer) Lines(n int) []string {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	total := len(rb.lines)
	// Include partial line if non-empty
	hasPartial := rb.partial != ""
	if hasPartial {
		total++
	}

	if n <= 0 || n > total {
		n = total
	}

	result := make([]string, 0, n)

	// Determine start index
	startFromLines := len(rb.lines)
	if hasPartial {
		startFromLines = len(rb.lines) + 1
	}
	skip := startFromLines - n

	for i := 0; i < len(rb.lines); i++ {
		if i >= skip {
			result = append(result, rb.lines[i])
		}
	}

	if hasPartial && skip < startFromLines {
		result = append(result, rb.partial)
	}

	return result
}
