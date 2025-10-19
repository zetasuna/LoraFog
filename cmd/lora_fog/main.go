// Package main is the entry point of the LoraFog system.
// It initializes the logger, loads the configuration, constructs all components
// (FogServer, Gateways, Vehicles) and starts them in a unified runtime.
package main

import (
	"LoraFog/internal/core"
	"LoraFog/internal/util"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
)

// main is the single entrypoint for the LoraFog application.
// It loads configuration, constructs the system and starts all components.
// The program waits for an interrupt signal and performs graceful shutdown.
func main() {
	util.SetupLogger()

	// cfgPath := "configs/config.yml"
	// Allow dynamic config path via CLI flag
	cfgPath := flag.String("c", "configs/config.yml", "path to configuration file")
	flag.Parse()

	log.Printf("[Main] Using config: %s", *cfgPath)

	// Initialize system
	sys, err := core.NewSystem(*cfgPath)
	if err != nil {
		log.Fatalf("failed to create system: %v", err)
	}

	if err := sys.StartAll(); err != nil {
		log.Fatalf("failed to start system: %v", err)
	}

	// wait for Ctrl+C or SIGTERM
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	log.Println("[Main] Shutting down system...")
	sys.StopAll()
	log.Println("[Main] System stopped cleanly.")
}
