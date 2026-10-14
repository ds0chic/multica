package util

import (
	"log/slog"
	"os"
	"strings"
)

// ParseBool reads the boolean spellings people put in .env files:
// true/1/yes/on/t/y and false/0/no/off/f/n, case-insensitive and trimmed.
// ok is false for anything else, including an empty string.
func ParseBool(raw string) (value bool, ok bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "t", "true", "y", "yes", "on":
		return true, true
	case "0", "f", "false", "n", "no", "off":
		return false, true
	}
	return false, false
}

// EnvBool reads a boolean environment variable with ParseBool. Unset or empty
// returns def; an unrecognized value logs a warning and returns def, so a
// typo never silently flips a setting to the opposite of what was meant.
func EnvBool(name string, def bool) bool {
	raw := os.Getenv(name)
	if strings.TrimSpace(raw) == "" {
		return def
	}
	if v, ok := ParseBool(raw); ok {
		return v
	}
	slog.Warn("unrecognized boolean env value, using default", "name", name, "value", raw, "default", def)
	return def
}
