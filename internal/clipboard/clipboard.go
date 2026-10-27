// Package clipboard defines the OS-independent clipboard boundary.
package clipboard

import "context"

const HardLimit = 1024 * 1024

type Event struct {
	Text      string
	Self      bool
	Sensitive bool
	// Sequence identifies an observed ownership event, not an observation of text.
	Sequence uint64
}

type Status struct {
	Supported bool   `json:"supported"`
	Healthy   bool   `json:"healthy"`
	Backend   string `json:"backend"`
	Detail    string `json:"detail"`
}

type Adapter interface {
	Start(context.Context, func(Event)) error
	ReadText(context.Context) (string, error)
	WriteText(context.Context, string) error
	Status() Status
	Stop() error
}
