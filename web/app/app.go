package app

import (
	"context"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.etcd.io/bbolt"
)

type App struct {
	DB     *bbolt.DB
	Tmpl   *template.Template
	Mux    *http.ServeMux
	Server *http.Server
}

// NewApp initializes the web app with templates, database, and routes.
func NewApp() (*App, error) {
	cwd, _ := os.Getwd()
	tmplPath := filepath.Join(cwd, "web", "templates", "*.html")

	tmpl := template.New("").Funcs(template.FuncMap{
		"year": func() int { return time.Now().Year() },
	})

	tmpl, err := tmpl.ParseGlob(tmplPath)
	if err != nil {
		return nil, fmt.Errorf("[app] failed to load templates: %w", err)
	}

	if err := os.MkdirAll("tmp", 0o755); err != nil {
		return nil, fmt.Errorf("[app] failed to create tmp/: %w", err)
	}

	dbPath := filepath.Join("tmp", "data.db")
	db, err := bbolt.Open(dbPath, 0o666, &bbolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("[app] failed to open BoltDB: %w", err)
	}

	app := &App{
		DB:   db,
		Tmpl: tmpl,
		Mux:  http.NewServeMux(),
	}

	app.registerRoutes()
	return app, nil
}

// Start launches the web server and blocks until stopped.
func (a *App) Start(addr string) error {
	if addr == "" {
		log.Println("[app] app server not started (empty address)")
		return nil
	}

	if a == nil {
		return fmt.Errorf("[app] Start called on nil receiver")
	}
	if a.Mux == nil {
		return fmt.Errorf("[app] nil HTTP mux — did you call registerRoutes()?")
	}

	addr = strings.TrimPrefix(addr, "http://")
	addr = strings.TrimPrefix(addr, "https://")
	if !strings.Contains(addr, ":") {
		addr = ":" + addr
	}

	a.Server = &http.Server{
		Addr:    addr,
		Handler: a.Mux,
	}

	log.Printf("[app] Web server listening at http://%s", addr)

	// Run server until Shutdown() is called
	if err := a.Server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("[app] HTTP server error: %w", err)
	}
	return nil
}

// Stop gracefully stops the web server and closes the DB.
func (a *App) Stop() {
	if a == nil {
		return
	}

	// Gracefully stop HTTP server
	if a.Server != nil {
		log.Println("[app] Shutting down web server...")
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := a.Server.Shutdown(ctx); err != nil {
			log.Printf("[app] HTTP server shutdown error: %v", err)
		} else {
			log.Println("[app] Web server stopped cleanly")
		}
	}

	// Close DB
	if a.DB != nil {
		if err := a.DB.Close(); err != nil {
			log.Printf("[app] error closing BoltDB: %v", err)
		} else {
			log.Println("[app] Closed BoltDB connection")
		}
	}
}
