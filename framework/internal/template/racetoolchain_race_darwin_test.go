//go:build race && darwin

package template

func init() { raceOnDarwin = true }
