package config

import (
	"errors"
)

type Settings struct {
	Monitoring    bool `json:"monitoring"`
	MaxEntries    int  `json:"maxEntries"`
	RetentionDays int  `json:"retentionDays"`
	MaxEntryBytes int  `json:"maxEntryBytes"`
}

func Default() Settings { return Settings{true, 1000, 30, 100 * 1024} }
func (s Settings) Validate() error {
	if s.MaxEntries < 1 || s.MaxEntries > 100000 {
		return errors.New("history limit must be between 1 and 100,000")
	}
	if s.RetentionDays < 1 || s.RetentionDays > 3650 {
		return errors.New("retention must be between 1 and 3,650 days")
	}
	if s.MaxEntryBytes < 1024 || s.MaxEntryBytes > 1024*1024 {
		return errors.New("entry size must be between 1 KB and 1 MB")
	}
	return nil
}
