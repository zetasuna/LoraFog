// Package app implements the web server and API layer for the LoraFog dashboard.
package app

import (
	"bytes"
	"io"
	"log"
	"net/http"
	"time"

	"go.etcd.io/bbolt"
)

// handleTelemetry stores incoming telemetry data into BoltDB.
func (a *App) handleTelemetry(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read telemetry", http.StatusBadRequest)
		return
	}
	if cerr := r.Body.Close(); cerr != nil {
		log.Printf("[app] warning: failed to close telemetry body: %v", cerr)
	}

	timestamp := time.Now().Format(time.RFC3339Nano)
	err = a.DB.Update(func(tx *bbolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte("telemetry"))
		if err != nil {
			return err
		}
		return b.Put([]byte(timestamp), body)
	})
	if err != nil {
		http.Error(w, "failed to save telemetry", http.StatusInternalServerError)
		return
	}

	log.Printf("[app] received telemetry (%d bytes)", len(body))
	w.WriteHeader(http.StatusOK)
}

// handleLatest retrieves the latest telemetry entry.
func (a *App) handleLatest(w http.ResponseWriter, r *http.Request) {
	err := a.DB.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte("telemetry"))
		if b == nil {
			http.Error(w, "no telemetry data", http.StatusNotFound)
			return nil
		}
		c := b.Cursor()
		k, v := c.Last()
		if v == nil {
			http.Error(w, "no data available", http.StatusNotFound)
			return nil
		}
		w.Header().Set("Content-Type", "application/json")
		if _, werr := w.Write(v); werr != nil {
			log.Printf("[app] warning: failed to write telemetry: %v", werr)
		}
		log.Printf("[app] latest telemetry @ %s", string(k))
		return nil
	})
	if err != nil {
		http.Error(w, "failed to read telemetry", http.StatusInternalServerError)
	}
}

// handleControl forwards a control command to the Fog server.
func (a *App) handleControl(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if cerr := r.Body.Close(); cerr != nil {
			log.Printf("[app] warning: failed to close control body: %v", cerr)
		}
	}()

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read control command", http.StatusBadRequest)
		return
	}

	// Forwards to FogServer (assuming it runs at :10000)
	resp, err := http.Post("http://localhost:10000/api/control", "application/json", bytes.NewReader(body))
	if err != nil {
		http.Error(w, "failed to forward control", http.StatusBadGateway)
		return
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			log.Printf("[app] warning: failed to close fog response: %v", cerr)
		}
	}()

	if _, err := io.Copy(io.Discard, resp.Body); err != nil {
		log.Printf("[app] warning: failed to drain fog response body: %v", err)
	}

	w.WriteHeader(http.StatusAccepted)
}
