// Package bindings exposes plain-Go service methods that the Wails v3 GUI
// (cmd/app, build tag "gui") binds to the frontend.
//
// It never imports Wails, so the service layer is unit-testable headless and
// the domain stays free of Wails dependencies (SPEC §3.3, rule 14). The
// methods return real application state, never fabricated measurements
// (SPEC rule 15/16).
package bindings

import "time"

// AppService exposes application-level calls to the UI.
type AppService struct {
	version string
	started time.Time
}

// NewAppService returns an AppService reporting the given build version.
func NewAppService(version string) *AppService {
	return &AppService{version: version, started: time.Now().UTC()}
}

// Version returns the build version string.
func (s *AppService) Version() string { return s.version }

// Health reports basic application health.
func (s *AppService) Health() HealthStatus {
	return HealthStatus{
		Ok:        true,
		Version:   s.version,
		StartedAt: s.started.Format(time.RFC3339),
	}
}

// HealthStatus is the DTO returned by AppService.Health.
type HealthStatus struct {
	Ok        bool   `json:"ok"`
	Version   string `json:"version"`
	StartedAt string `json:"startedAt"`
}
