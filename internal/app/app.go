package app

import (
	"cliprag/internal/categorizer"
	"cliprag/internal/clipboard"
	"cliprag/internal/config"
	"cliprag/internal/history"
	"cliprag/internal/storage"
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

type queuedEvent struct {
	event clipboard.Event
	epoch uint64
}
type Engine struct {
	store        *storage.Store
	adapter      clipboard.Adapter
	dir          string
	mu           sync.Mutex
	settings     config.Settings
	lastSequence uint64
	lastError    string
	epoch        atomic.Uint64
	enabled      atomic.Bool
	cancel       context.CancelFunc
	done         chan struct{}
	notify       func()
	queue        chan queuedEvent
}

func New(store *storage.Store, adapter clipboard.Adapter, dir string, notify func()) (*Engine, error) {
	cfg, err := store.Settings(context.Background())
	if err != nil {
		return nil, err
	}
	e := &Engine{store: store, adapter: adapter, dir: dir, settings: cfg, notify: notify, queue: make(chan queuedEvent, 64)}
	e.enabled.Store(cfg.Monitoring)
	return e, nil
}
func (e *Engine) Start(ctx context.Context) error {
	child, cancel := context.WithCancel(ctx)
	e.cancel = cancel
	e.done = make(chan struct{})
	go func() {
		defer close(e.done)
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-child.Done():
				return
			case q := <-e.queue:
				if q.epoch == e.epoch.Load() {
					e.captureAt(child, q.event, q.epoch)
				}
			case <-ticker.C:
				e.mu.Lock()
				cfg := e.settings
				e.mu.Unlock()
				if err := e.store.Cleanup(child, cfg, time.Now().UnixMilli()); err != nil {
					e.failure("Retention cleanup failed")
				} else {
					e.changed()
				}
			}
		}
	}()
	if err := e.store.Cleanup(ctx, e.settings, time.Now().UnixMilli()); err != nil {
		e.failure("Retention cleanup failed")
	}
	if err := e.adapter.Start(child, func(v clipboard.Event) {
		if !e.enabled.Load() {
			return
		}
		q := queuedEvent{v, e.epoch.Load()}
		select {
		case e.queue <- q:
		default:
			e.failure("Capture queue full; some rapid clipboard changes were skipped")
		}
	}); err != nil {
		e.failure(err.Error())
		return err
	}
	return nil
}
func (e *Engine) Stop() error {
	if e.cancel == nil {
		return nil
	}
	e.cancel()
	err := e.adapter.Stop()
	<-e.done
	return err
}
func (e *Engine) changed() {
	if e.notify != nil {
		e.notify()
	}
}
func (e *Engine) failure(message string) {
	e.mu.Lock()
	e.lastError = message
	e.mu.Unlock()
	e.changed()
}
func (e *Engine) capture(ctx context.Context, v clipboard.Event) { e.captureAt(ctx, v, e.epoch.Load()) }
func (e *Engine) captureAt(ctx context.Context, v clipboard.Event, epoch uint64) {
	e.mu.Lock()
	if epoch != e.epoch.Load() {
		e.mu.Unlock()
		return
	}
	if !e.settings.Monitoring {
		e.mu.Unlock()
		return
	}
	if v.Sequence != 0 && v.Sequence <= e.lastSequence {
		e.mu.Unlock()
		return
	}
	e.lastSequence = v.Sequence
	if v.Self {
		e.mu.Unlock()
		return
	}
	cfg := e.settings
	if history.Eligible(v.Text, v.Sensitive, cfg.MaxEntryBytes) != nil {
		e.mu.Unlock()
		return
	}
	// Serialize policy changes with persistence, so pause/clear cannot race capture.
	err := e.store.Capture(ctx, v.Text, categorizer.Classify(v.Text), time.Now().UnixMilli(), cfg)
	if err != nil {
		e.lastError = "Could not save clipboard history"
	}
	e.mu.Unlock()
	e.changed()
}
func (e *Engine) copy(ctx context.Context, id int64) error {
	item, err := e.store.Get(ctx, id)
	if err != nil {
		return err
	}
	if err = e.adapter.WriteText(ctx, item.Content); err != nil {
		return errors.New("clipboard restore failed; check the desktop bridge")
	}
	if err = e.store.Restore(ctx, id); err != nil {
		return err
	}
	e.changed()
	return nil
}
