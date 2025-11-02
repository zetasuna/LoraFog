package app

import (
	"log"
	"net/http"
)

// handleDashboard renders the main dashboard page.
func (a *App) handleDashboard(w http.ResponseWriter, r *http.Request) {
	log.Printf("[app] GET / (dashboard) from %s", r.RemoteAddr)
	data := map[string]any{
		"Title": "LoraFog Dashboard",
	}
	if err := a.Tmpl.ExecuteTemplate(w, "dashboard.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// handleGateways renders the list of gateways.
func (a *App) handleGateways(w http.ResponseWriter, r *http.Request) {
	data := map[string]any{"Title": "Gateways"}
	if err := a.Tmpl.ExecuteTemplate(w, "gateways.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// handleVehicles renders the list of vehicles.
func (a *App) handleVehicles(w http.ResponseWriter, r *http.Request) {
	data := map[string]any{"Title": "Vehicles"}
	if err := a.Tmpl.ExecuteTemplate(w, "vehicles.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
