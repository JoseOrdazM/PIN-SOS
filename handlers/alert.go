package handlers

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
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

// MaxUploadBytes caps the whole multipart body (form + photo).
const MaxUploadBytes = 12 << 20 // 12 MB

// allowedImageTypes maps the REAL sniffed content type to the extension we
// store. Extension from the client's filename is never trusted.
var allowedImageTypes = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/webp": ".webp",
}

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

func jsonError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "error": msg})
}

// savePhoto validates the uploaded file by sniffing its real content type
// and stores it under a random, unguessable filename. Returns the stored
// path or "" (photo is optional; a bad photo aborts the request instead of
// being silently dropped, so the sender knows it did not go through).
func (h *AlertHandler) savePhoto(r *http.Request) (string, error) {
	file, _, err := r.FormFile("photo")
	if err != nil {
		return "", nil // no photo attached — fine
	}
	defer file.Close()

	// Sniff the real type from the first 512 bytes.
	head := make([]byte, 512)
	n, err := io.ReadFull(file, head)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return "", fmt.Errorf("read photo: %w", err)
	}
	contentType := http.DetectContentType(head[:n])
	ext, ok := allowedImageTypes[contentType]
	if !ok {
		return "", fmt.Errorf("tipo de archivo no permitido: %s (solo JPG, PNG o WebP)", contentType)
	}

	// Random filename: uploaded photos are served without auth, so the URL
	// must be unguessable (timestamps are enumerable).
	var rb [16]byte
	if _, err := rand.Read(rb[:]); err != nil {
		return "", fmt.Errorf("rand: %w", err)
	}
	filename := hex.EncodeToString(rb[:]) + ext
	destPath := filepath.Join(h.uploadsDir, filename)

	dst, err := os.Create(destPath)
	if err != nil {
		return "", fmt.Errorf("create file: %w", err)
	}
	defer dst.Close()

	if _, err := dst.Write(head[:n]); err != nil {
		os.Remove(destPath)
		return "", fmt.Errorf("write photo: %w", err)
	}
	if _, err := io.Copy(dst, file); err != nil {
		os.Remove(destPath)
		return "", fmt.Errorf("write photo: %w", err)
	}
	return destPath, nil
}

func (h *AlertHandler) Create(w http.ResponseWriter, r *http.Request) {
	// Hard cap on the request body BEFORE parsing anything.
	r.Body = http.MaxBytesReader(w, r.Body, MaxUploadBytes)

	if err := r.ParseMultipartForm(10 << 20); err != nil {
		jsonError(w, http.StatusBadRequest, "Formulario inválido o archivo demasiado grande (máx. 10 MB)")
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	phone := strings.TrimSpace(r.FormValue("phone"))
	description := strings.TrimSpace(r.FormValue("description"))
	latStr := strings.TrimSpace(r.FormValue("lat"))
	lngStr := strings.TrimSpace(r.FormValue("lng"))

	if name == "" || phone == "" || description == "" || latStr == "" || lngStr == "" {
		jsonError(w, http.StatusBadRequest, "Faltan campos obligatorios: nombre, teléfono, descripción y ubicación")
		return
	}

	// Bound field lengths: this is user-supplied data that ends up in the
	// panel and in Telegram.
	if len(name) > 200 || len(phone) > 50 || len(description) > 2000 {
		jsonError(w, http.StatusBadRequest, "Campo demasiado largo")
		return
	}

	lat, err := strconv.ParseFloat(latStr, 64)
	if err != nil || lat < -90 || lat > 90 {
		jsonError(w, http.StatusBadRequest, "Latitud inválida")
		return
	}
	lng, err := strconv.ParseFloat(lngStr, 64)
	if err != nil || lng < -180 || lng > 180 {
		jsonError(w, http.StatusBadRequest, "Longitud inválida")
		return
	}

	mapsLink := fmt.Sprintf("https://maps.google.com/?q=%.6f,%.6f", lat, lng)

	photoPath, err := h.savePhoto(r)
	if err != nil {
		log.Printf("photo rejected: %v", err)
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}

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
		jsonError(w, http.StatusInternalServerError, "Error interno del servidor")
		return
	}

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

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"ok":        true,
		"id":        alertID,
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

// auth compares the bearer token in constant time to avoid timing leaks.
func (h *PanelHandler) auth(r *http.Request) bool {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		return false
	}
	got := strings.TrimPrefix(auth, "Bearer ")
	return subtle.ConstantTimeCompare([]byte(got), []byte(h.panelToken)) == 1
}

func (h *PanelHandler) List(w http.ResponseWriter, r *http.Request) {
	if !h.auth(r) {
		jsonError(w, http.StatusUnauthorized, "Unauthorized")
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
		jsonError(w, http.StatusInternalServerError, "Error interno del servidor")
		return
	}

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
		jsonError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "Invalid alert ID")
		return
	}

	var body struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}

	if err := h.db.UpdateStatus(id, body.Status); err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"ok":     true,
		"id":     id,
		"status": body.Status,
	})
}
