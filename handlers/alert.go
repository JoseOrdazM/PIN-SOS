package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/JoseOrdazM/PIN-SOS/db"
	"github.com/JoseOrdazM/PIN-SOS/telegram"
)

type AlertHandler struct {
	db         *db.DB
	notifier   *telegram.Notifier
	uploadsDir string
}

func NewAlertHandler(database *db.DB, notifier *telegram.Notifier, uploadsDir string) *AlertHandler {
	return &AlertHandler{
		db:         database,
		notifier:   notifier,
		uploadsDir: uploadsDir,
	}
}

func (h *AlertHandler) Create(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(10 << 20); err != nil { // 10MB max
		http.Error(w, `{"ok":false,"error":"Failed to parse form"}`, http.StatusBadRequest)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	phone := strings.TrimSpace(r.FormValue("phone"))
	description := strings.TrimSpace(r.FormValue("description"))
	latStr := strings.TrimSpace(r.FormValue("lat"))
	lngStr := strings.TrimSpace(r.FormValue("lng"))

	// Validate required fields
	if name == "" || phone == "" || description == "" || latStr == "" || lngStr == "" {
		http.Error(w, `{"ok":false,"error":"Missing required fields: name, phone, description, lat, lng"}`, http.StatusBadRequest)
		return
	}

	lat, err := strconv.ParseFloat(latStr, 64)
	if err != nil {
		http.Error(w, `{"ok":false,"error":"Invalid latitude"}`, http.StatusBadRequest)
		return
	}

	lng, err := strconv.ParseFloat(lngStr, 64)
	if err != nil {
		http.Error(w, `{"ok":false,"error":"Invalid longitude"}`, http.StatusBadRequest)
		return
	}

	mapsLink := fmt.Sprintf("https://maps.google.com/?q=%.6f,%.6f", lat, lng)

	// Handle optional photo
	var photoPath string
	file, header, err := r.FormFile("photo")
	if err == nil {
		defer file.Close()
		ext := filepath.Ext(header.Filename)
		if ext == "" {
			ext = ".jpg"
		}
		filename := fmt.Sprintf("%d%s", time.Now().UnixNano(), ext)
		destPath := filepath.Join(h.uploadsDir, filename)

		dst, err := os.Create(destPath)
		if err != nil {
			log.Printf("Failed to create file %s: %v", destPath, err)
		} else {
			defer dst.Close()
			if _, err := io.Copy(dst, file); err == nil {
				photoPath = destPath
			}
		}
	}

	// Create alert record
	alert := &db.Alert{
		Name:        name,
		Phone:       phone,
		Description: description,
		Lat:         lat,
		Lng:         lng,
		MapsLink:    mapsLink,
		PhotoPath:   photoPath,
	}

	alertID, err := h.db.CreateAlert(alert)
	if err != nil {
		log.Printf("Failed to create alert: %v", err)
		http.Error(w, `{"ok":false,"error":"Internal server error"}`, http.StatusInternalServerError)
		return
	}

	// Notify Telegram
	createdAt := time.Now().UTC().Format("02/01/2006 15:04 UTC")
	alertIDStr := strconv.FormatInt(alertID, 10)

	var photo *telegram.Photo
	if photoPath != "" {
		photo = &telegram.Photo{Path: photoPath}
	}

	go func() {
		if err := h.notifier.SendAlert(name, phone, description, mapsLink, alertIDStr, createdAt, photo); err != nil {
			log.Printf("Telegram notification failed: %v", err)
		}
	}()

	// Response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"ok":       true,
		"id":       alertID,
		"maps_link": mapsLink,
	})
}

type PanelHandler struct {
	db         *db.DB
	panelToken string
}

func NewPanelHandler(database *db.DB, panelToken string) *PanelHandler {
	return &PanelHandler{
		db:         database,
		panelToken: panelToken,
	}
}

func (h *PanelHandler) auth(r *http.Request) bool {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		return false
	}
	return strings.TrimPrefix(auth, "Bearer ") == h.panelToken
}

func (h *PanelHandler) List(w http.ResponseWriter, r *http.Request) {
	if !h.auth(r) {
		http.Error(w, `{"ok":false,"error":"Unauthorized"}`, http.StatusUnauthorized)
		return
	}

	status := r.URL.Query().Get("status")
	limitStr := r.URL.Query().Get("limit")

	limit := 0
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	alerts, err := h.db.ListAlerts(status, limit)
	if err != nil {
		log.Printf("Failed to list alerts: %v", err)
		http.Error(w, `{"ok":false,"error":"Internal server error"}`, http.StatusInternalServerError)
		return
	}

	// Convert photo path to URL path for response
	for i := range alerts {
		if alerts[i].PhotoPath != "" {
			alerts[i].PhotoPath = "/uploads/" + filepath.Base(alerts[i].PhotoPath)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"ok":     true,
		"alerts": alerts,
	})
}

func (h *PanelHandler) UpdateStatus(w http.ResponseWriter, r *http.Request) {
	if !h.auth(r) {
		http.Error(w, `{"ok":false,"error":"Unauthorized"}`, http.StatusUnauthorized)
		return
	}

	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, `{"ok":false,"error":"Invalid alert ID"}`, http.StatusBadRequest)
		return
	}

	var body struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"ok":false,"error":"Invalid JSON body"}`, http.StatusBadRequest)
		return
	}

	if err := h.db.UpdateStatus(id, body.Status); err != nil {
		http.Error(w, fmt.Sprintf(`{"ok":false,"error":"%s"}`, err.Error()), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"ok":     true,
		"id":     id,
		"status": body.Status,
	})
}
