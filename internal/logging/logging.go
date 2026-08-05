// Package logging writes append-only, human-readable application events.
package logging

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Logger is the event sink used by Dockwarden.
type Logger interface {
	Log(level, event string, fields map[string]string) error
}

// TextLogger writes one escaped event per line.
type TextLogger struct {
	mu     sync.Mutex
	writer io.Writer
	closer io.Closer
}

// NewWriter creates a logger backed by writer. It does not close writer.
func NewWriter(writer io.Writer) *TextLogger {
	return &TextLogger{writer: writer}
}

// NewFile opens path for append and restricts the file to the current user.
func NewFile(path string) (*TextLogger, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("log file path is empty")
	}
	if directory := filepath.Dir(path); directory != "." {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			return nil, fmt.Errorf("cannot create log directory: %w", err)
		}
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("cannot open log file %s: %w", path, err)
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("cannot restrict log file %s: %w", path, err)
	}
	return &TextLogger{writer: file, closer: file}, nil
}

// Log writes a timestamped event. Field values are quoted and escaped.
func (l *TextLogger) Log(level, event string, fields map[string]string) error {
	if l == nil || l.writer == nil {
		return nil
	}
	level = strings.ToUpper(strings.TrimSpace(level))
	if level == "" {
		level = "INFO"
	}
	event = strings.TrimSpace(event)
	if event == "" {
		event = "unnamed"
	}

	keys := make([]string, 0, len(fields))
	for key := range fields {
		if strings.TrimSpace(key) != "" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)

	var line strings.Builder
	fmt.Fprintf(&line, "%s level=%s event=%s", time.Now().UTC().Format(time.RFC3339Nano), level, event)
	for _, key := range keys {
		fmt.Fprintf(&line, " %s=%s", key, strconv.Quote(fields[key]))
	}
	line.WriteByte('\n')

	l.mu.Lock()
	defer l.mu.Unlock()
	_, err := io.WriteString(l.writer, line.String())
	return err
}

// Close closes a file-backed logger. Writer-backed loggers do nothing.
func (l *TextLogger) Close() error {
	if l == nil || l.closer == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.closer.Close()
}
