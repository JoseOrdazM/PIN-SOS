package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/JoseOrdazM/PIN-SOS/db"
	"github.com/JoseOrdazM/PIN-SOS/handlers"
	"github.com/JoseOrdazM/PIN-SOS/telegram"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "/data/pinsos.db"
	}

	uploadsDir := os.Getenv("UPLOADS_DIR")
	if uploadsDir == "" {
		uploadsDir = "/data/uploads"
	}

	panelToken := os.Getenv("PANEL_TOKEN")
	if panelToken == "" {
		log.Fatal("PANEL_TOKEN env var required")
	}

	botToken := os.Getenv("TELEGRAM_BOT_TOKEN")
	chatID := os.Getenv("TELEGRAM_CHAT_ID")

	// Ensure dirs exist
	if err := os.MkdirAll(uploadsDir, 0755); err != nil {
		log.Fatalf("Failed to create uploads dir: %v", err)
	}

	// Init DB
	database, err := db.Init(dbPath)
	if err != nil {
		log.Fatalf("Failed to init DB: %v", err)
	}
	defer database.Close()

	// Init Telegram notifier
	notifier := telegram.New(botToken, chatID)

	// Setup handlers
	alertHandler := handlers.NewAlertHandler(database, notifier, uploadsDir)
	panelHandler := handlers.NewPanelHandler(database, panelToken)

	mux := http.NewServeMux()

	// API endpoints
	mux.HandleFunc("POST /api/alert", alertHandler.Create)
	mux.HandleFunc("GET /api/alerts", panelHandler.List)
	mux.HandleFunc("PATCH /api/alerts/{id}/status", panelHandler.UpdateStatus)

	// Serve uploaded files
	mux.Handle("GET /uploads/", http.StripPrefix("/uploads/",
		http.FileServer(http.Dir(uploadsDir))))

	// Static files
	staticDir := os.Getenv("STATIC_DIR")
	if staticDir == "" {
		staticDir = "static"
	}
	mux.Handle("GET /", http.FileServer(http.Dir(staticDir)))

	// Start server
	addr := ":" + port
	log.Printf("PIN-SOS v2 starting on %s", addr)
	log.Printf("DB: %s", filepath.Join(dbPath))
	log.Printf("Uploads: %s", uploadsDir)

	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
