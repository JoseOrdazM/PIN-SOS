package telegram

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type Notifier struct {
	botToken string
	chatID   string
	enabled  bool
}

type Photo struct {
	Path string
}

func New(botToken, chatID string) *Notifier {
	enabled := botToken != "" && chatID != ""
	if !enabled {
		log.Println("Telegram notifier disabled: missing bot token or chat ID")
	}
	return &Notifier{
		botToken: botToken,
		chatID:   chatID,
		enabled:  enabled,
	}
}

func (n *Notifier) SendAlert(name, phone, description, mapsLink, alertID, createdAt string, photo *Photo) error {
	if !n.enabled {
		log.Println("Telegram disabled, skipping notification")
		return nil
	}

	text := fmt.Sprintf(`🚨 *ALERTA SOS VENEZUELA* 🚨

👤 *Nombre:* %s
📞 *Teléfono:* %s
📝 *Descripción:* %s
📍 *Ubicación:* %s
🕐 *Hora:* %s
🔢 *ID:* #%s`,
		escapeMarkdown(name),
		escapeMarkdown(phone),
		escapeMarkdown(description),
		mapsLink,
		escapeMarkdown(createdAt),
		escapeMarkdown(alertID),
	)

	// Send text message
	if err := n.sendMessage(text); err != nil {
		return fmt.Errorf("send text: %w", err)
	}

	// Send photo if exists
	if photo != nil && photo.Path != "" {
		if err := n.sendPhoto(photo.Path); err != nil {
			log.Printf("Warning: failed to send photo: %v", err)
		}
	}

	return nil
}

func (n *Notifier) sendMessage(text string) error {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", n.botToken)

	payload := map[string]interface{}{
		"chat_id":                  n.chatID,
		"text":                     text,
		"parse_mode":               "Markdown",
		"disable_web_page_preview": true,
	}

	body, _ := json.Marshal(payload)
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("http post: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("telegram API error %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

func (n *Notifier) sendPhoto(photoPath string) error {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendPhoto", n.botToken)

	file, err := os.Open(photoPath)
	if err != nil {
		return fmt.Errorf("open photo: %w", err)
	}
	defer file.Close()

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	writer.WriteField("chat_id", n.chatID)

	part, err := writer.CreateFormFile("photo", filepath.Base(photoPath))
	if err != nil {
		return fmt.Errorf("create form file: %w", err)
	}

	if _, err := io.Copy(part, file); err != nil {
		return fmt.Errorf("copy photo: %w", err)
	}
	writer.Close()

	resp, err := http.Post(url, writer.FormDataContentType(), &buf)
	if err != nil {
		return fmt.Errorf("http post: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("telegram photo API error %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

func escapeMarkdown(s string) string {
	// Escape markdown special characters
	replacer := strings.NewReplacer(
		"_", "\\_",
		"*", "\\*",
		"`", "\\`",
		"[", "\\[",
		"]", "\\]",
	)
	return replacer.Replace(s)
}
