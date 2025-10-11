// Gateway program:
// - Reads CSV from LoRa (/dev/serial0)
// - Parses CSV -> JSON and forwards to Fog /ingest (worker pool)
// - Exposes /command endpoint (JSON) to receive control from Fog; converts to CSV and sends via LoRa
// - Registers itself to Fog on startup (POST /register)
package main

import (
	"LoraFog/internal/lora"
	"LoraFog/internal/model"
	"LoraFog/internal/parser"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"time"
)

func main() {
	serialDev := flag.String("lora", "/dev/serial0", "LoRa serial device")
	baud := flag.Int("baud", 9600, "serial baud")
	fogAddr := flag.String("fog", "http://192.168.2.245:3001", "Fog base URL")
	gatewayID := flag.String("id", "GW01", "gateway id")
	flag.Parse()

	// open LoRa
	l, err := lora.New(*serialDev, *baud)
	if err != nil {
		log.Fatalf("open lora: %v", err)
	}
	defer func() {
		if cerr := l.Close(); cerr != nil {
			log.Printf("warning: close lora err: %v", cerr)
		}
	}()

	// channels and worker pools
	forwardCh := make(chan model.VehicleData, 1024)
	sendCh := make(chan string, 512)
	var wg sync.WaitGroup

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// forward workers
	client := &http.Client{Timeout: 5 * time.Second}
	for i := range 4 {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for v := range forwardCh {
				// b, _ := json.Marshal(v)
				// resp, err := client.Post(*fogAddr+"/ingest", "application/json", bytes.NewReader(b))
				// // Convert struct -> CSV string
				csvLine := parser.VehicleToCSV(v)
				resp, err := client.Post(*fogAddr+"/api/telemetry", "text/plain", strings.NewReader(csvLine))
				if err != nil {
					log.Printf("forward worker %d post err: %v", id, err)
					continue
				}
				_, _ = io.Copy(io.Discard, resp.Body)
				if cerr := resp.Body.Close(); cerr != nil {
					log.Printf("warning: forward worker %d close body: %v", id, cerr)
				}
			}
		}(i)
	}

	// send workers
	for i := range 2 {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for s := range sendCh {
				if err := l.WriteLine(s); err != nil {
					log.Printf("send worker %d write err: %v", id, err)
				}
				time.Sleep(30 * time.Millisecond)
			}
		}(i)
	}

	// register to Fog
	reg := model.GatewayRegistration{
		GatewayID: *gatewayID,
		URL:       "http://127.0.0.1:10001", // gateway's own /command endpoint; in production set external reachable URL
	}
	breg, _ := json.Marshal(reg)
	_, err = client.Post(*fogAddr+"/register", "application/json", bytes.NewReader(breg))
	if err != nil {
		log.Printf("warning: failed register to fog: %v", err)
	} else {
		log.Printf("registered to fog as %s", reg.GatewayID)
	}

	// HTTP /command endpoint to accept ControlMessage from Fog (or admin)
	http.HandleFunc("/command", func(w http.ResponseWriter, r *http.Request) {
		var ctl model.ControlMessage
		if err := json.NewDecoder(r.Body).Decode(&ctl); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		// convert to CSV: VEHICLE_ID,PAYLOAD,MSGID
		csv := parser.ControlToCSV(ctl)
		select {
		case sendCh <- csv:
			w.WriteHeader(http.StatusAccepted)
			if _, err := w.Write([]byte("enqueued")); err != nil {
				log.Printf("failed to write HTTP response: %v", err)
			}
		default:
			http.Error(w, "send queue full", http.StatusServiceUnavailable)
		}
	})

	// goroutine: read serial, parse telemetry, enqueue forward
	go func() {
		for {
			line, err := l.ReadLine(0)
			if err != nil {
				// nonfatal: retry
				time.Sleep(100 * time.Millisecond)
				continue
			}
			vd, err := parser.ParseTelemetryCSV(line)
			if err != nil {
				log.Printf("parse err: %v (line=%s)", err, line)
				continue
			}
			select {
			case forwardCh <- vd:
			default:
				log.Println("forward queue full, drop")
			}
		}
	}()

	// start http server
	srv := &http.Server{Addr: ":10001"}
	go func() {
		log.Println("gateway http listening :10001")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("gateway http err: %v", err)
		}
	}()

	// wait signal
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)
	<-stop
	log.Println("gateway shutting down...")

	// shutdown http server
	_ = srv.Shutdown(ctx)

	// close channels and wait
	close(sendCh)
	close(forwardCh)
	wg.Wait()
	cancel()
	log.Println("gateway stopped")
}
