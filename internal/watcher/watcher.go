package watcher

import (
	"context"

	watcherEvents "github.com/yusing/godoxy/internal/watcher/events"
)

type Event = watcherEvents.Event

type Watcher interface {
	Events(ctx context.Context) (<-chan Event, <-chan error)
}
