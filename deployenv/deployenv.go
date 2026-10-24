// Package deployenv answers one question for clients that talk to real
// vendors (factoring, PrePass): is this process the real production
// deployment?
//
// The answer comes only from RAILWAY_ENVIRONMENT_NAME, which Railway sets on
// every service itself — nobody types it, so a copied settings block or a typo
// cannot change it. APP_ENV is deliberately NOT consulted: it is typed by hand
// per service and has drifted (staging accounting ran with APP_ENV=prod and
// dialed the real Triumph server, DEV-3306).
//
// Production means the name is exactly "production". Everything else — dev,
// staging, a copied or renamed environment, a laptop, CI (no variable at all)
// — is not production, so a mistake fails safe.
package deployenv

import "os"

const (
	// RailwayEnvVar is the variable Railway sets to the environment's name.
	RailwayEnvVar = "RAILWAY_ENVIRONMENT_NAME"

	productionName = "production"
)

// IsProduction reports whether real vendor servers may be used.
func IsProduction() bool {
	return Name() == productionName
}

// Name returns the raw Railway environment name ("" off Railway), for error
// messages that explain why a real server was refused.
func Name() string {
	return os.Getenv(RailwayEnvVar)
}
