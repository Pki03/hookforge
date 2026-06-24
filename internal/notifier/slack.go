package notifier

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

type SlackMessage struct {
	Text string `json:"text"`
}

func SendSlackAlert(webhookURL, eventID, endpointID, status string, attempts, maxRetries int) {
	if webhookURL == "" {
		return
	}

	msg := fmt.Sprintf("🚨 HookForge Alert\n*Event:* `%s`\n*Endpoint:* `%s`\n*Status:* %s\n*Attempts:* %d/%d\n*Time:* %s",
		eventID, endpointID, status, attempts, maxRetries, time.Now().UTC().Format(time.RFC3339))

	body, _ := json.Marshal(SlackMessage{Text: msg})

	resp, err := http.Post(webhookURL, "application/json", bytes.NewReader(body))
	if err != nil {
		slog.Error("slack notify error", "error", err)
		return
	}
	resp.Body.Close()
	slog.Info("slack alert sent", "event_id", eventID[:min(len(eventID), 8)])
}
