package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/JoseOrdazM/PIN-SOS/db"
	"github.com/JoseOrdazM/PIN-SOS/handlers"
	"github.com/JoseOrdazM/PIN-SOS/middleware"
	"github.com/JoseOrdazM/PIN-SOS/telegram"
)

func getEnv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func getEnvInt(k string, d int) int {
	if v := os.Getenv(k); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return d
}

// retentionLoop deletes CERRADA alerts older than retentionDays, including
// their photo files, once every 12 hours.
func retentionLoop(database *db.DB, retentionDays int) {
	run := func() {
		photos, err := database.DeleteOldClosed(retentionDays)
		if err != nil {
			log.Printf("retention: %v", err)
			return
		}
		for _, p := range photos {
			if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
				log.Printf("retention: remove photo %s: %v", p, err)
			}
		}
		if len(photos) > 0 {
			log.Printf("retention: purged closed alerts older than %d days (%d photos removed)", retentionDays, len(photos))
		}
	}
	run() // once at startup
	t := time.NewTicker(12 * time.Hour)
	defer t.Stop()
	for range t.C {
		run()
	}
}

// uploadsHandler serves uploaded photos with headers that prevent them from
// being rendered as active content (an SVG or HTML disguised as an image
// must never execute in the panel operator's browser).
func uploadsHandler(uploadsDir string) http.Handler {
	fs := http.StripPrefix("/uploads/", http.FileServer(http.Dir(uploadsDir)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; img-src 'self'; style-src 'unsafe-inline'; sandbox")
		fs.ServeHTTP(w, r)
	})
}

func main() {
	port := getEnv("PORT", "8080")
	dbPath := getEnv("DB_PATH", "/data/pinsos.db")
	uploadsDir := getEnv("UPLOADS_DIR", "/data/uploads")
	staticDir := getEnv("STATIC_DIR", "static")
	retentionDays := getEnvInt("RETENTION_DAYS", 30)

	panelToken := os.Getenv("PANEL_TOKEN")
	if panelToken == "" {
		log.Fatal("PANEL_TOKEN env var required")
	}
	if len(panelToken) < 32 {
		log.Fatal("PANEL_TOKEN too short: use at least 32 chars (openssl rand -hex 32)")
	}

	botToken := os.Getenv("TELEGRAM_BOT_TOKEN")
	chatID := os.Getenv("TELEGRAM_CHAT_ID")

	if err := os.MkdirAll(uploadsDir, 0o755); err != nil {
		log.Fatalf("Failed to create uploads dir: %v", err)
	}

	database, err := db.Init(dbPath)
	if err != nil {
		log.Fatalf("Failed to init DB: %v", err)
	}
	defer database.Close()

	notifier := telegram.New(botToken, chatID)

	alertHandler := handlers.NewAlertHandler(database, notifier, uploadsDir)
	panelHandler := handlers.NewPanelHandler(database, panelToken)

	// Rate limits: creating an alert is expensive (disk + Telegram) and
	// public. Defaults: 5 alerts/min per IP, 60/min globally. Tunable by env.
	alertLimiter := middleware.NewRateLimiter(
		getEnvInt("RATE_PER_IP", 5),
		getEnvInt("RATE_GLOBAL", 60),
		time.Minute,
	)
	// Panel auth attempts also limited to slow down token brute force.
	panelLimiter := middleware.NewRateLimiter(
		getEnvInt("PANEL_RATE_PER_IP", 60),
		getEnvInt("PANEL_RATE_GLOBAL", 600),
		time.Minute,
	)

	mux := http.NewServeMux()

	// API endpoints
	mux.HandleFunc("POST /api/alert", alertLimiter.Middleware(alertHandler.Create))
	mux.HandleFunc("GET /api/alerts", panelLimiter.Middleware(panelHandler.List))
	mux.HandleFunc("PATCH /api/alerts/{id}/status", panelLimiter.Middleware(panelHandler.UpdateStatus))

	// Uploaded files (unguessable random names; never rendered as active content)
	mux.Handle("GET /uploads/", uploadsHandler(uploadsDir))

	// Static files
	mux.Handle("GET /", http.FileServer(http.Dir(staticDir)))

	// Data retention
	go retentionLoop(database, retentionDays)

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       2 * time.Minute, // photo uploads on slow mobile networks
		WriteTimeout:      2 * time.Minute,
		IdleTimeout:       2 * time.Minute,
		MaxHeaderBytes:    16 << 10,
	}

	log.Printf("PIN-SOS v2 starting on :%s", port)
	log.Printf("DB: %s | Uploads: %s | Retention: %d days", filepath.Clean(dbPath), uploadsDir, retentionDays)

	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
