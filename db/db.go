package db

import (
	"database/sql"
	"fmt"

	_ "github.com/mattn/go-sqlite3"
)

type Alert struct {
	ID          int64  `json:"id"`
	CreatedAt   string `json:"created_at"`
	Name        string `json:"name"`
	Phone       string `json:"phone"`
	Description string `json:"description"`
	Lat         float64 `json:"lat"`
	Lng         float64 `json:"lng"`
	MapsLink    string `json:"maps_link"`
	PhotoPath   string `json:"photo_path,omitempty"`
	Status      string `json:"status"`
}

type DB struct {
	conn *sql.DB
}

func Init(dbPath string) (*DB, error) {
	conn, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_foreign_keys=on")
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	if err := conn.Ping(); err != nil {
		return nil, fmt.Errorf("ping db: %w", err)
	}

	schema := `
	CREATE TABLE IF NOT EXISTS alerts (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		name TEXT NOT NULL,
		phone TEXT NOT NULL,
		description TEXT NOT NULL,
		lat REAL NOT NULL,
		lng REAL NOT NULL,
		maps_link TEXT NOT NULL,
		photo_path TEXT,
		status TEXT DEFAULT 'ACTIVA'
	);
	CREATE INDEX IF NOT EXISTS idx_alerts_status ON alerts(status);
	CREATE INDEX IF NOT EXISTS idx_alerts_created ON alerts(created_at DESC);
	`

	if _, err := conn.Exec(schema); err != nil {
		return nil, fmt.Errorf("create schema: %w", err)
	}

	return &DB{conn: conn}, nil
}

func (d *DB) Close() error {
	return d.conn.Close()
}

func (d *DB) CreateAlert(a *Alert) (int64, error) {
	result, err := d.conn.Exec(`
		INSERT INTO alerts (name, phone, description, lat, lng, maps_link, photo_path)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, a.Name, a.Phone, a.Description, a.Lat, a.Lng, a.MapsLink, a.PhotoPath)
	if err != nil {
		return 0, fmt.Errorf("insert alert: %w", err)
	}
	return result.LastInsertId()
}

func (d *DB) ListAlerts(status string, limit int) ([]Alert, error) {
	query := "SELECT id, created_at, name, phone, description, lat, lng, maps_link, COALESCE(photo_path,''), status FROM alerts"
	args := make([]interface{}, 0)

	if status != "" {
		query += " WHERE status = ?"
		args = append(args, status)
	}

	query += " ORDER BY created_at DESC"

	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}

	rows, err := d.conn.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list alerts: %w", err)
	}
	defer rows.Close()

	var alerts []Alert
	for rows.Next() {
		var a Alert
		if err := rows.Scan(&a.ID, &a.CreatedAt, &a.Name, &a.Phone,
			&a.Description, &a.Lat, &a.Lng, &a.MapsLink, &a.PhotoPath, &a.Status); err != nil {
			return nil, fmt.Errorf("scan alert: %w", err)
		}
		alerts = append(alerts, a)
	}
	return alerts, rows.Err()
}

func (d *DB) GetAlert(id int64) (*Alert, error) {
	var a Alert
	err := d.conn.QueryRow(`
		SELECT id, created_at, name, phone, description, lat, lng, maps_link, COALESCE(photo_path,''), status
		FROM alerts WHERE id = ?
	`, id).Scan(&a.ID, &a.CreatedAt, &a.Name, &a.Phone,
		&a.Description, &a.Lat, &a.Lng, &a.MapsLink, &a.PhotoPath, &a.Status)
	if err != nil {
		return nil, fmt.Errorf("get alert %d: %w", id, err)
	}
	return &a, nil
}

func (d *DB) UpdateStatus(id int64, status string) error {
	valid := map[string]bool{"ACTIVA": true, "ATENDIDA": true, "CERRADA": true}
	if !valid[status] {
		return fmt.Errorf("invalid status: %s", status)
	}
	result, err := d.conn.Exec("UPDATE alerts SET status = ? WHERE id = ?", status, id)
	if err != nil {
		return fmt.Errorf("update status: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("alert %d not found", id)
	}
	return nil
}
