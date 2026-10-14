package selfhosttelemetry

import (
	"log/slog"

	"github.com/multica-ai/multica/server/internal/util"
)

// Config is read once during API server startup. Endpoint selection is
// deliberately absent: self-host telemetry can only be sent to Multica's
// first-party collector compiled into the client.
type Config struct {
	Enabled bool
}

// ConfigFromDoNotTrack implements the industry-standard opt-out switch used by
// the self-hosted server. Any true spelling (1, true, yes, on; see
// util.ParseBool) disables telemetry; every other value preserves the
// default-on behavior.
func ConfigFromDoNotTrack(raw string) Config {
	disabled, ok := util.ParseBool(raw)
	return Config{Enabled: !(ok && disabled)}
}

// LogStartupStatus makes the default-on behavior observable without logging
// the raw environment value or any telemetry data.
func LogStartupStatus(logger *slog.Logger, config Config) {
	if config.Enabled {
		logger.Info("self-host telemetry enabled")
		return
	}
	logger.Info("self-host telemetry disabled via DO_NOT_TRACK")
}
