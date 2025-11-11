package slack

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Notifier handles sending alerts to Slack
type Notifier struct {
	webhookURL string
	httpClient *http.Client
}

// Message represents a Slack message payload
type Message struct {
	Text        string       `json:"text,omitempty"`
	Attachments []Attachment `json:"attachments,omitempty"`
}

// Attachment represents a Slack message attachment
type Attachment struct {
	Color  string  `json:"color,omitempty"`
	Title  string  `json:"title,omitempty"`
	Text   string  `json:"text,omitempty"`
	Fields []Field `json:"fields,omitempty"`
	Footer string  `json:"footer,omitempty"`
	Ts     int64   `json:"ts,omitempty"`
}

// Field represents a field in a Slack attachment
type Field struct {
	Title string `json:"title"`
	Value string `json:"value"`
	Short bool   `json:"short"`
}

// NewNotifier creates a new Slack notifier
func NewNotifier(webhookURL string) *Notifier {
	return &Notifier{
		webhookURL: webhookURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// SendError sends an error notification to Slack
func (n *Notifier) SendError(component, errorMsg string) error {
	msg := Message{
		Attachments: []Attachment{
			{
				Color: "danger",
				Title: fmt.Sprintf("Octopus Monitor Error - %s", component),
				Text:  errorMsg,
				Fields: []Field{
					{
						Title: "Component",
						Value: component,
						Short: true,
					},
					{
						Title: "Time",
						Value: time.Now().Format(time.RFC3339),
						Short: true,
					},
				},
				Footer: "Octopus Home Mini Monitor",
				Ts:     time.Now().Unix(),
			},
		},
	}

	return n.send(msg)
}

// SendWarning sends a warning notification to Slack
func (n *Notifier) SendWarning(component, warningMsg string) error {
	msg := Message{
		Attachments: []Attachment{
			{
				Color: "warning",
				Title: fmt.Sprintf("Octopus Monitor Warning - %s", component),
				Text:  warningMsg,
				Fields: []Field{
					{
						Title: "Component",
						Value: component,
						Short: true,
					},
					{
						Title: "Time",
						Value: time.Now().Format(time.RFC3339),
						Short: true,
					},
				},
				Footer: "Octopus Home Mini Monitor",
				Ts:     time.Now().Unix(),
			},
		},
	}

	return n.send(msg)
}

// SendInfo sends an informational notification to Slack
func (n *Notifier) SendInfo(title, message string) error {
	msg := Message{
		Attachments: []Attachment{
			{
				Color: "good",
				Title: title,
				Text:  message,
				Fields: []Field{
					{
						Title: "Time",
						Value: time.Now().Format(time.RFC3339),
						Short: true,
					},
				},
				Footer: "Octopus Home Mini Monitor",
				Ts:     time.Now().Unix(),
			},
		},
	}

	return n.send(msg)
}

// SendCacheAlert sends an alert about cached data
func (n *Notifier) SendCacheAlert(count int, action string) error {
	msg := Message{
		Attachments: []Attachment{
			{
				Color: "warning",
				Title: "Cache Status Update",
				Text:  fmt.Sprintf("%s: %d data points in cache", action, count),
				Fields: []Field{
					{
						Title: "Action",
						Value: action,
						Short: true,
					},
					{
						Title: "Data Points",
						Value: fmt.Sprintf("%d", count),
						Short: true,
					},
					{
						Title: "Time",
						Value: time.Now().Format(time.RFC3339),
						Short: false,
					},
				},
				Footer: "Octopus Home Mini Monitor",
				Ts:     time.Now().Unix(),
			},
		},
	}

	return n.send(msg)
}

// send sends a message to Slack via webhook
func (n *Notifier) send(msg Message) error {
	payload, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	resp, err := n.httpClient.Post(n.webhookURL, "application/json", bytes.NewBuffer(payload))
	if err != nil {
		return fmt.Errorf("failed to send message to Slack: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Slack returned non-OK status: %d", resp.StatusCode)
	}

	return nil
}
