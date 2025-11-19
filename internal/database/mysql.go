// Package database provides MySQL-backed storage for vehicle/gateway mapping.
package database

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

type ServerDB struct {
	db *sql.DB
}

// NewServerDB opens a MySQL connection using DSN (user:pass@tcp(host:port)/dbname)
func NewServerDB(dsn string, maxOpen, maxIdle int) (*ServerDB, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(maxOpen)
	db.SetMaxIdleConns(maxIdle)
	db.SetConnMaxLifetime(5 * time.Minute)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}
	slog.Info("MySQL connected", "dsn", dsn)
	return &ServerDB{db: db}, nil
}

// Close closes underlying DB connection.
func (s *ServerDB) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// UpdateVehicleGateway sets gateway_id for a boat (vehicle).
// If gatewayID == "" -> sets NULL. Uses context with timeout.
func (s *ServerDB) UpdateVehicleGateway(ctx context.Context, vehicleID, gatewayID string) error {
	if s == nil || s.db == nil {
		return errors.New("db not initialized")
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	var q string
	if gatewayID == "" {
		q = `UPDATE boats SET gateway_id = NULL WHERE boatId = ?`
		_, err := s.db.ExecContext(ctx, q, vehicleID)
		if err != nil {
			slog.Error("UpdateVehicleGateway failed", "vehicle", vehicleID, "error", err)
			return err
		}
		slog.Info("DB: Vehicle unregistered", "vehicle", vehicleID)
		return nil
	}

	q = `UPDATE boats SET gateway_id = ? WHERE boatId = ?`
	_, err := s.db.ExecContext(ctx, q, gatewayID, vehicleID)
	if err != nil {
		slog.Error("UpdateVehicleGateway failed", "vehicle", vehicleID, "gateway", gatewayID, "error", err)
		return err
	}
	slog.Info("DB: Vehicle registered", "vehicle", vehicleID, "gateway", gatewayID)
	return nil
}

// GetVehicleGateway returns gatewayId for given vehicle boatId. Returns "" if none.
// UPDATED v2
func (s *ServerDB) GetVehicleGateway(ctx context.Context, vehicleID string) (string, error) {
	if s == nil || s.db == nil {
		return "", errors.New("db not initialized")
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	var gw sql.NullString
	q := `SELECT gateway_id FROM boats WHERE boatId = ? LIMIT 1`
	err := s.db.QueryRowContext(ctx, q, vehicleID).Scan(&gw)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	if gw.Valid {
		return gw.String, nil
	}
	return "", nil
}

// ClearVehicleGateway sets gateway_id to NULL for vehicle.
// UPDATED v2
func (s *ServerDB) ClearVehicleGateway(ctx context.Context, vehicleID string) error {
	return s.UpdateVehicleGateway(ctx, vehicleID, "")
}
