package sim

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
)

func TestShouldFailUsesConfiguredRate(t *testing.T) {
	config := &Config{}

	config.Update(0, 0, 0)
	for range 100 {
		if config.ShouldFail() {
			t.Fatal("zero failure rate produced a failure")
		}
	}

	config.Update(1, 0, 0)
	for range 100 {
		if !config.ShouldFail() {
			t.Fatal("100% failure rate produced a success")
		}
	}
}

func TestRegisterUIServesConfigurationPage(t *testing.T) {
	app := fiber.New()
	RegisterUI(app)

	response, err := app.Test(httptest.NewRequest("GET", "/", nil))
	if err != nil {
		t.Fatalf("request root UI: %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", response.StatusCode)
	}
	if contentType := response.Header.Get("Content-Type"); contentType != fiber.MIMETextHTMLCharsetUTF8 {
		t.Fatalf("content type = %q, want %q", contentType, fiber.MIMETextHTMLCharsetUTF8)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	page := string(body)
	if !strings.Contains(page, "Simulation configuration") {
		t.Fatal("root response does not contain the configuration UI")
	}
	if !strings.Contains(page, `id="reset"`) || !strings.Contains(page, "/admin/reset/") {
		t.Fatal("root response does not contain the reset state action")
	}
	if !strings.Contains(page, "Transactions") || !strings.Contains(page, "Pending webhooks") {
		t.Fatal("root response does not contain the in-memory activity views")
	}
	if !strings.Contains(page, `id="theme"`) || !strings.Contains(page, `value="system" selected`) {
		t.Fatal("root response does not contain the system-default theme switch")
	}
}

func TestFireWebhookIsTrackedWhilePending(t *testing.T) {
	ResetPendingWebhooks()
	started := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		<-release
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	defer close(release)
	defer ResetPendingWebhooks()

	FireWebhook(server.URL, map[string]string{"referenceId": "tx-123"})
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("webhook delivery did not start")
	}

	pending := PendingWebhooks()
	if len(pending) != 1 {
		t.Fatalf("pending webhook count = %d, want 1", len(pending))
	}
	if pending[0].URL != server.URL {
		t.Fatalf("pending webhook URL = %q, want %q", pending[0].URL, server.URL)
	}
	var payload map[string]string
	if err := json.Unmarshal(pending[0].Payload, &payload); err != nil {
		t.Fatalf("decode pending payload: %v", err)
	}
	if payload["referenceId"] != "tx-123" {
		t.Fatalf("pending webhook payload = %#v", payload)
	}

	release <- struct{}{}
	deadline := time.Now().Add(time.Second)
	for len(PendingWebhooks()) != 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if pending := PendingWebhooks(); len(pending) != 0 {
		t.Fatalf("pending webhook count after delivery = %d, want 0", len(pending))
	}
}
