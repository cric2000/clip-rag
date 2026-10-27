package app

import (
	"cliprag/internal/clipboard"
	"cliprag/internal/config"
	"cliprag/internal/history"
	"context"
	"time"
)

// Service is the bounded desktop IPC surface. It is independent of Wails.
type Service struct{ engine *Engine }

func NewService(e *Engine) *Service { return &Service{e} }

type State struct {
	Clipboard  clipboard.Status `json:"clipboard"`
	Monitoring bool             `json:"monitoring"`
	Error      string           `json:"error"`
	DataDir    string           `json:"dataDir"`
}

func (s *Service) List(q history.Query) (history.Page, error) {
	return s.engine.store.List(context.Background(), q)
}
func (s *Service) Counts() (map[string]int, error) {
	return s.engine.store.Counts(context.Background())
}
func (s *Service) Status() State {
	e := s.engine
	e.mu.Lock()
	state := State{Monitoring: e.settings.Monitoring, Error: e.lastError, DataDir: e.dir}
	e.mu.Unlock()
	state.Clipboard = e.adapter.Status()
	return state
}
func (s *Service) GetSettings() config.Settings {
	s.engine.mu.Lock()
	defer s.engine.mu.Unlock()
	return s.engine.settings
}
func (s *Service) SaveSettings(c config.Settings) error {
	if err := c.Validate(); err != nil {
		return err
	}
	e := s.engine
	e.mu.Lock()
	if err := e.store.SaveSettings(context.Background(), c); err != nil {
		e.mu.Unlock()
		return err
	}
	e.settings = c
	e.epoch.Add(1)
	e.enabled.Store(c.Monitoring)
	e.mu.Unlock()
	e.changed()
	return nil
}
func (s *Service) Copy(id int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return s.engine.copy(ctx, id)
}
func (s *Service) Pin(id int64, pinned bool) error {
	e := s.engine
	e.mu.Lock()
	err := e.store.Pin(context.Background(), id, pinned)
	if err == nil {
		err = e.store.Cleanup(context.Background(), e.settings, time.Now().UnixMilli())
	}
	e.mu.Unlock()
	if err == nil {
		e.changed()
	}
	return err
}
func (s *Service) Delete(id int64) error {
	e := s.engine
	e.mu.Lock()
	e.epoch.Add(1)
	err := e.store.Delete(context.Background(), id)
	e.mu.Unlock()
	if err == nil {
		e.changed()
	}
	return err
}
func (s *Service) Clear(confirmation string) error {
	e := s.engine
	e.mu.Lock()
	e.epoch.Add(1)
	err := e.store.Clear(context.Background(), confirmation)
	e.mu.Unlock()
	if err == nil {
		e.changed()
	}
	return err
}
