package health

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewServer(t *testing.T) {
	server := NewServer(":8080", "1.0.0")

	if server == nil {
		t.Fatal("NewServer returned nil")
	}

	if server.addr != ":8080" {
		t.Errorf("addr = %v, want :8080", server.addr)
	}

	if server.version != "1.0.0" {
		t.Errorf("version = %v, want 1.0.0", server.version)
	}
}

func TestHealthHandler(t *testing.T) {
	server := NewServer(":8080", "1.0.0")

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	server.healthHandler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status code = %v, want %v", w.Code, http.StatusOK)
	}

	var response HealthResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response.Status != StatusHealthy {
		t.Errorf("status = %v, want %v", response.Status, StatusHealthy)
	}

	if response.Version != "1.0.0" {
		t.Errorf("version = %v, want 1.0.0", response.Version)
	}
}

func TestReadinessHandler_AllHealthy(t *testing.T) {
	server := NewServer(":8080", "1.0.0")

	// Register healthy checkers
	server.RegisterChecker("component1", func(ctx context.Context) ComponentHealth {
		return ComponentHealth{
			Status:  StatusHealthy,
			Message: "component1 is healthy",
		}
	})

	server.RegisterChecker("component2", func(ctx context.Context) ComponentHealth {
		return ComponentHealth{
			Status:  StatusHealthy,
			Message: "component2 is healthy",
		}
	})

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	w := httptest.NewRecorder()

	server.readinessHandler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status code = %v, want %v", w.Code, http.StatusOK)
	}

	var response ReadinessResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if !response.Ready {
		t.Error("ready should be true")
	}

	if len(response.Components) != 2 {
		t.Errorf("components count = %v, want 2", len(response.Components))
	}
}

func TestReadinessHandler_OneUnhealthy(t *testing.T) {
	server := NewServer(":8080", "1.0.0")

	// Register one healthy and one unhealthy checker
	server.RegisterChecker("healthy_component", func(ctx context.Context) ComponentHealth {
		return ComponentHealth{
			Status:  StatusHealthy,
			Message: "healthy",
		}
	})

	server.RegisterChecker("unhealthy_component", func(ctx context.Context) ComponentHealth {
		return ComponentHealth{
			Status:  StatusUnhealthy,
			Message: "unhealthy",
		}
	})

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	w := httptest.NewRecorder()

	server.readinessHandler(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status code = %v, want %v", w.Code, http.StatusServiceUnavailable)
	}

	var response ReadinessResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response.Ready {
		t.Error("ready should be false")
	}
}

func TestReadinessHandler_NoCheckers(t *testing.T) {
	server := NewServer(":8080", "1.0.0")

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	w := httptest.NewRecorder()

	server.readinessHandler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status code = %v, want %v", w.Code, http.StatusOK)
	}

	var response ReadinessResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if !response.Ready {
		t.Error("ready should be true with no checkers")
	}
}

func TestSimpleChecker(t *testing.T) {
	// Test healthy checker
	healthyChecker := SimpleChecker("test", func() error {
		return nil
	})

	health := healthyChecker(context.Background())
	if health.Status != StatusHealthy {
		t.Errorf("status = %v, want %v", health.Status, StatusHealthy)
	}

	// Test unhealthy checker
	unhealthyChecker := SimpleChecker("test", func() error {
		return fmt.Errorf("test error")
	})

	health = unhealthyChecker(context.Background())
	if health.Status != StatusUnhealthy {
		t.Errorf("status = %v, want %v", health.Status, StatusUnhealthy)
	}
}

func TestContextChecker(t *testing.T) {
	// Test healthy checker
	healthyChecker := ContextChecker("test", func(ctx context.Context) error {
		return nil
	})

	health := healthyChecker(context.Background())
	if health.Status != StatusHealthy {
		t.Errorf("status = %v, want %v", health.Status, StatusHealthy)
	}

	// Test unhealthy checker
	unhealthyChecker := ContextChecker("test", func(ctx context.Context) error {
		return fmt.Errorf("test error")
	})

	health = unhealthyChecker(context.Background())
	if health.Status != StatusUnhealthy {
		t.Errorf("status = %v, want %v", health.Status, StatusUnhealthy)
	}

	// Test context cancellation
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	timeoutChecker := ContextChecker("test", func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	})

	health = timeoutChecker(ctx)
	if health.Status != StatusUnhealthy {
		t.Errorf("status = %v, want %v", health.Status, StatusUnhealthy)
	}
}

func TestRegisterChecker(t *testing.T) {
	server := NewServer(":8080", "1.0.0")

	checker := func(ctx context.Context) ComponentHealth {
		return ComponentHealth{Status: StatusHealthy}
	}

	server.RegisterChecker("test", checker)

	if len(server.checkers) != 1 {
		t.Errorf("checkers count = %v, want 1", len(server.checkers))
	}

	if _, exists := server.checkers["test"]; !exists {
		t.Error("checker not registered")
	}
}

func TestServer_StartStop(t *testing.T) {
	server := NewServer(":0", "1.0.0") // Use port 0 for random available port

	err := server.Start()
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = server.Stop(ctx)
	if err != nil {
		t.Errorf("Stop() error = %v", err)
	}
}

func TestReadinessHandler_ContextTimeout(t *testing.T) {
	server := NewServer(":8080", "1.0.0")

	// Register a slow checker
	server.RegisterChecker("slow", func(ctx context.Context) ComponentHealth {
		select {
		case <-time.After(10 * time.Second):
			return ComponentHealth{Status: StatusHealthy}
		case <-ctx.Done():
			return ComponentHealth{
				Status:  StatusUnhealthy,
				Message: "timeout",
			}
		}
	})

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	server.readinessHandler(w, req)

	var response ReadinessResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// The checker should have been called with the context
	if response.Ready {
		t.Error("ready should be false due to timeout")
	}
}

func TestStatus_String(t *testing.T) {
	tests := []struct {
		status Status
		want   string
	}{
		{StatusHealthy, "healthy"},
		{StatusUnhealthy, "unhealthy"},
		{StatusDegraded, "degraded"},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			if string(tt.status) != tt.want {
				t.Errorf("status string = %v, want %v", string(tt.status), tt.want)
			}
		})
	}
}

func TestConcurrentCheckerRegistration(t *testing.T) {
	server := NewServer(":8080", "1.0.0")

	done := make(chan bool, 10)

	// Register checkers concurrently
	for i := 0; i < 10; i++ {
		go func(id int) {
			checker := func(ctx context.Context) ComponentHealth {
				return ComponentHealth{Status: StatusHealthy}
			}
			server.RegisterChecker(fmt.Sprintf("checker_%d", id), checker)
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	if len(server.checkers) != 10 {
		t.Errorf("checkers count = %v, want 10", len(server.checkers))
	}
}
