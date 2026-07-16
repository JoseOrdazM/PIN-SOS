package handlers

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JoseOrdazM/PIN-SOS/db"
	"github.com/JoseOrdazM/PIN-SOS/telegram"
)

func testHandler(t *testing.T) (*AlertHandler, *PanelHandler, string) {
	t.Helper()
	dir := t.TempDir()
	database, err := db.Init(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("init db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	uploads := filepath.Join(dir, "uploads")
	os.MkdirAll(uploads, 0o755)
	notifier := telegram.New("", "") // disabled
	return NewAlertHandler(database, notifier, uploads),
		NewPanelHandler(database, "test-token-test-token-test-token"), uploads
}

// multipartBody builds a multipart form with the given fields and optional
// photo bytes.
func multipartBody(t *testing.T, fields map[string]string, photo []byte, photoName string) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for k, v := range fields {
		w.WriteField(k, v)
	}
	if photo != nil {
		fw, err := w.CreateFormFile("photo", photoName)
		if err != nil {
			t.Fatalf("create form file: %v", err)
		}
		fw.Write(photo)
	}
	w.Close()
	return &buf, w.FormDataContentType()
}

func validFields() map[string]string {
	return map[string]string{
		"name":        "María Pérez",
		"phone":       "+58 412 0000000",
		"description": "Necesito ayuda",
		"lat":         "10.4806",
		"lng":         "-66.9036",
	}
}

func pngBytes(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	img.Set(0, 0, color.RGBA{255, 0, 0, 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

func TestCreateAlert(t *testing.T) {
	cases := []struct {
		name      string
		fields    map[string]string
		photo     []byte
		photoName string
		wantCode  int
	}{
		{"ok without photo", validFields(), nil, "", http.StatusCreated},
		{"ok with real png", validFields(), nil, "", http.StatusCreated},
		{"missing name", func() map[string]string { f := validFields(); delete(f, "name"); return f }(), nil, "", http.StatusBadRequest},
		{"bad latitude", func() map[string]string { f := validFields(); f["lat"] = "999"; return f }(), nil, "", http.StatusBadRequest},
		{"bad longitude", func() map[string]string { f := validFields(); f["lng"] = "abc"; return f }(), nil, "", http.StatusBadRequest},
		{"oversized field", func() map[string]string { f := validFields(); f["name"] = strings.Repeat("a", 300); return f }(), nil, "", http.StatusBadRequest},
		{"fake image (html disguised as jpg)", validFields(), []byte("<html><script>alert(1)</script></html>"), "evil.jpg", http.StatusBadRequest},
	}
	cases[1].photo = pngBytes(t)
	cases[1].photoName = "foto.png"

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h, _, uploads := testHandler(t)
			body, ct := multipartBody(t, tc.fields, tc.photo, tc.photoName)
			req := httptest.NewRequest("POST", "/api/alert", body)
			req.Header.Set("Content-Type", ct)
			rec := httptest.NewRecorder()

			h.Create(rec, req)

			if rec.Code != tc.wantCode {
				t.Fatalf("want %d, got %d: %s", tc.wantCode, rec.Code, rec.Body.String())
			}
			if tc.wantCode == http.StatusCreated && tc.photo != nil {
				// The stored file must exist with a random name and .png ext.
				entries, _ := os.ReadDir(uploads)
				if len(entries) != 1 {
					t.Fatalf("want 1 stored photo, got %d", len(entries))
				}
				name := entries[0].Name()
				if !strings.HasSuffix(name, ".png") || len(name) < 32 {
					t.Fatalf("stored name not random+sniffed-ext: %s", name)
				}
			}
		})
	}
}

func TestFakeImageIsNotStored(t *testing.T) {
	h, _, uploads := testHandler(t)
	body, ct := multipartBody(t, validFields(), []byte("MZ\x90\x00 not an image"), "malware.png")
	req := httptest.NewRequest("POST", "/api/alert", body)
	req.Header.Set("Content-Type", ct)
	rec := httptest.NewRecorder()
	h.Create(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rec.Code)
	}
	entries, _ := os.ReadDir(uploads)
	if len(entries) != 0 {
		t.Fatalf("rejected file must not be stored, found %d files", len(entries))
	}
}

func TestPanelAuth(t *testing.T) {
	_, p, _ := testHandler(t)

	cases := []struct {
		name     string
		header   string
		wantCode int
	}{
		{"no header", "", http.StatusUnauthorized},
		{"wrong token", "Bearer nope", http.StatusUnauthorized},
		{"wrong scheme", "Basic test-token-test-token-test-token", http.StatusUnauthorized},
		{"correct token", "Bearer test-token-test-token-test-token", http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/alerts", nil)
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}
			rec := httptest.NewRecorder()
			p.List(rec, req)
			if rec.Code != tc.wantCode {
				t.Fatalf("want %d, got %d", tc.wantCode, rec.Code)
			}
		})
	}
}

func TestUpdateStatusValidation(t *testing.T) {
	h, p, _ := testHandler(t)

	// Create one alert first.
	body, ct := multipartBody(t, validFields(), nil, "")
	req := httptest.NewRequest("POST", "/api/alert", body)
	req.Header.Set("Content-Type", ct)
	rec := httptest.NewRecorder()
	h.Create(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("setup create failed: %d", rec.Code)
	}
	var created struct {
		ID int64 `json:"id"`
	}
	json.Unmarshal(rec.Body.Bytes(), &created)

	patch := func(id, status string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("PATCH", "/api/alerts/"+id+"/status",
			strings.NewReader(`{"status":"`+status+`"}`))
		req.SetPathValue("id", id)
		req.Header.Set("Authorization", "Bearer test-token-test-token-test-token")
		rec := httptest.NewRecorder()
		p.UpdateStatus(rec, req)
		return rec
	}

	if rec := patch("1", "ATENDIDA"); rec.Code != http.StatusOK {
		t.Fatalf("valid status: want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec := patch("1", "HACKED"); rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid status: want 400, got %d", rec.Code)
	}
	if rec := patch("9999", "CERRADA"); rec.Code != http.StatusBadRequest {
		t.Fatalf("missing id: want 400, got %d", rec.Code)
	}
}
