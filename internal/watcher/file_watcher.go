package watcher

import (
	"context"
)

type fileWatcher struct {
	relPath string
	eventCh chan Event
	errCh   chan error
}

var _ Watcher = (*fileWatcher)(nil)

// Events implements the Watcher interface.
func (fw *fileWatcher) Events(ctx context.Context) (<-chan Event, <-chan error) {
	return fw.eventCh, fw.errCh
}
