package slack

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewNotifier(t *testing.T) {
	webhookURL := "https://example.com/test-webhook"
	notifier := NewNotifier(webhookURL)

	if notifier == nil {
		t.Fatal("NewNotifier() returned nil")
	}

	if notifier.webhookURL != webhookURL {
		t.Errorf("webhookURL = %v, want %v", notifier.webhookURL, webhookURL)
	}

	if notifier.httpClient == nil {
		t.Error("httpClient is nil")
	}
}

func TestNotifier_SendError(t *testing.T) {
	tests := []struct {
		name         string
		component    string
		errorMsg     string
		serverStatus int
		serverBody   string
		wantErr      bool
		wantContains []string
	}{
		{
			name:         "successful error notification",
			component:    "TestComponent",
			errorMsg:     "Test error message",
			serverStatus: http.StatusOK,
			serverBody:   "ok",
			wantErr:      false,
			wantContains: []string{"TestComponent", "Test error message", "danger"},
		},
		{
			name:         "slack returns error",
			component:    "TestComponent",
			errorMsg:     "Test error message",
			serverStatus: http.StatusBadRequest,
			serverBody:   "error",
			wantErr:      true,
		},
		{
			name:         "slack returns server error",
			component:    "Database",
			errorMsg:     "Connection failed",
			serverStatus: http.StatusInternalServerError,
			serverBody:   "internal error",
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test server
			var receivedBody string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Read body
				buf := make([]byte, r.ContentLength)
				r.Body.Read(buf)
				receivedBody = string(buf)

				w.WriteHeader(tt.serverStatus)
				w.Write([]byte(tt.serverBody))
			}))
			defer server.Close()

			notifier := NewNotifier(server.URL)
			err := notifier.SendError(tt.component, tt.errorMsg)

			if tt.wantErr {
				if err == nil {
					t.Error("SendError() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("SendError() unexpected error = %v", err)
				}

				// Verify message contents
				for _, want := range tt.wantContains {
					if !strings.Contains(receivedBody, want) {
						t.Errorf("Message body does not contain %q. Body: %s", want, receivedBody)
					}
				}
			}
		})
	}
}

func TestNotifier_SendWarning(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	notifier := NewNotifier(server.URL)
	err := notifier.SendWarning("TestComponent", "Test warning")

	if err != nil {
		t.Errorf("SendWarning() unexpected error = %v", err)
	}
}

func TestNotifier_SendInfo(t *testing.T) {
	var receivedBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, r.ContentLength)
		r.Body.Read(buf)
		receivedBody = string(buf)

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	notifier := NewNotifier(server.URL)
	err := notifier.SendInfo("Test Title", "Test message")

	if err != nil {
		t.Errorf("SendInfo() unexpected error = %v", err)
	}

	if !strings.Contains(receivedBody, "Test Title") {
		t.Error("Message does not contain title")
	}

	if !strings.Contains(receivedBody, "good") {
		t.Error("Message does not have 'good' color")
	}
}

func TestNotifier_SendCacheAlert(t *testing.T) {
	tests := []struct {
		name         string
		count        int
		action       string
		wantContains []string
	}{
		{
			name:         "cache data added",
			count:        10,
			action:       "Data cached",
			wantContains: []string{"10", "Data cached"},
		},
		{
			name:         "cache synced",
			count:        50,
			action:       "Synced to InfluxDB",
			wantContains: []string{"50", "Synced to InfluxDB"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var receivedBody string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				buf := make([]byte, r.ContentLength)
				r.Body.Read(buf)
				receivedBody = string(buf)

				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			notifier := NewNotifier(server.URL)
			err := notifier.SendCacheAlert(tt.count, tt.action)

			if err != nil {
				t.Errorf("SendCacheAlert() unexpected error = %v", err)
			}

			for _, want := range tt.wantContains {
				if !strings.Contains(receivedBody, want) {
					t.Errorf("Message body does not contain %q", want)
				}
			}
		})
	}
}

func TestNotifier_NetworkError(t *testing.T) {
	// Use invalid URL to simulate network error
	notifier := NewNotifier("http://invalid-url-that-does-not-exist.local:9999")
	err := notifier.SendError("Test", "Test message")

	if err == nil {
		t.Error("Expected network error, got nil")
	}
}

func TestNotifier_InvalidJSON(t *testing.T) {
	// This test ensures the JSON marshaling works correctly
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify Content-Type
		contentType := r.Header.Get("Content-Type")
		if contentType != "application/json" {
			t.Errorf("Content-Type = %v, want application/json", contentType)
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	notifier := NewNotifier(server.URL)

	// Test with various special characters
	err := notifier.SendError("Component \"with\" quotes", "Error with\nnewlines\tand\ttabs")
	if err != nil {
		t.Errorf("SendError() with special characters error = %v", err)
	}
}

func TestMessage_Structure(t *testing.T) {
	msg := Message{
		Text: "Test message",
		Attachments: []Attachment{
			{
				Color: "good",
				Title: "Test Title",
				Text:  "Test Text",
				Fields: []Field{
					{
						Title: "Field 1",
						Value: "Value 1",
						Short: true,
					},
				},
				Footer: "Test Footer",
				Ts:     1234567890,
			},
		},
	}

	if msg.Text != "Test message" {
		t.Error("Message text not set correctly")
	}

	if len(msg.Attachments) != 1 {
		t.Errorf("Expected 1 attachment, got %d", len(msg.Attachments))
	}

	if msg.Attachments[0].Color != "good" {
		t.Error("Attachment color not set correctly")
	}

	if len(msg.Attachments[0].Fields) != 1 {
		t.Errorf("Expected 1 field, got %d", len(msg.Attachments[0].Fields))
	}
}
