package config

import "testing"

func TestValidation(t *testing.T) {
	if e := Default().Validate(); e != nil {
		t.Fatal(e)
	}
	for _, mutate := range []func(*Settings){func(c *Settings) { c.MaxEntries = 0 }, func(c *Settings) { c.MaxEntries = 100001 }, func(c *Settings) { c.RetentionDays = 0 }, func(c *Settings) { c.RetentionDays = 3651 }, func(c *Settings) { c.MaxEntryBytes = 1023 }, func(c *Settings) { c.MaxEntryBytes = 1048577 }} {
		c := Default()
		mutate(&c)
		if c.Validate() == nil {
			t.Fatal("invalid settings accepted")
		}
	}
}
