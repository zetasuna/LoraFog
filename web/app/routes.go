package app

import (
	"net/http"
)

// registerRoutes sets up all HTTP handlers for the application.
func (a *App) registerRoutes() {
	// Static files (CSS, JS)
	fs := http.FileServer(http.Dir("web/static"))
	a.Mux.Handle("/static/", http.StripPrefix("/static/", fs))

	// Public routes
	a.Mux.HandleFunc("/", a.handleDashboard)
	a.Mux.HandleFunc("/gateways", a.handleGateways)
	a.Mux.HandleFunc("/vehicles", a.handleVehicles)
	a.Mux.HandleFunc("/login", a.handleLogin)
	a.Mux.HandleFunc("/logout", a.handleLogout)

	// API routes
	a.Mux.HandleFunc("/api/telemetry", a.handleTelemetry)
	a.Mux.HandleFunc("/api/latest", a.handleLatest)
	a.Mux.HandleFunc("/api/control", a.handleControl)
}
