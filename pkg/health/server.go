package health

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

// Status represents the health status of a component
type Status string

const (
	StatusHealthy   Status = "healthy"
	StatusUnhealthy Status = "unhealthy"
	StatusDegraded  Status = "degraded"
)

// ComponentHealth represents the health of a single component
type ComponentHealth struct {
	Status  Status `json:"status"`
	Message string `json:"message,omitempty"`
}

// HealthResponse represents the overall health response
type HealthResponse struct {
	Status     Status                      `json:"status"`
	Timestamp  string                      `json:"timestamp"`
	Version    string                      `json:"version,omitempty"`
	Components map[string]ComponentHealth `json:"components,omitempty"`
}

// ReadinessResponse represents the readiness check response
type ReadinessResponse struct {
	Ready      bool                        `json:"ready"`
	Timestamp  string                      `json:"timestamp"`
	Components map[string]ComponentHealth `json:"components"`
}

// Checker is a function that checks the health of a component
type Checker func(ctx context.Context) ComponentHealth

// Server provides health check endpoints
type Server struct {
	addr     string
	server   *http.Server
	version  string
	checkers map[string]Checker
	mu       sync.RWMutex
}

// NewServer creates a new health check server
func NewServer(addr, version string) *Server {
	return &Server{
		addr:     addr,
		version:  version,
		checkers: make(map[string]Checker),
	}
}

// RegisterChecker registers a health checker for a component
func (s *Server) RegisterChecker(name string, checker Checker) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.checkers[name] = checker
}

// Start starts the health check HTTP server
func (s *Server) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.healthHandler)
	mux.HandleFunc("/ready", s.readinessHandler)

	s.server = &http.Server{
		Addr:         s.addr,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	log.Printf("Starting health check server on %s", s.addr)

	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("Health server error: %v", err)
		}
	}()

	return nil
}

// Stop gracefully stops the health check server
func (s *Server) Stop(ctx context.Context) error {
	if s.server == nil {
		return nil
	}

	log.Println("Stopping health check server...")
	return s.server.Shutdown(ctx)
}

// healthHandler handles the /health endpoint (liveness check)
func (s *Server) healthHandler(w http.ResponseWriter, r *http.Request) {
	// Basic liveness check - if we can respond, we're alive
	response := HealthResponse{
		Status:    StatusHealthy,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Version:   s.version,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// readinessHandler handles the /ready endpoint (readiness check)
func (s *Server) readinessHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	s.mu.RLock()
	checkers := make(map[string]Checker, len(s.checkers))
	for name, checker := range s.checkers {
		checkers[name] = checker
	}
	s.mu.RUnlock()

	components := make(map[string]ComponentHealth)
	ready := true

	// Check all components in parallel
	var wg sync.WaitGroup
	var mu sync.Mutex

	for name, checker := range checkers {
		wg.Add(1)
		go func(componentName string, check Checker) {
			defer wg.Done()

			health := check(ctx)

			mu.Lock()
			components[componentName] = health
			if health.Status == StatusUnhealthy {
				ready = false
			}
			mu.Unlock()
		}(name, checker)
	}

	wg.Wait()

	response := ReadinessResponse{
		Ready:      ready,
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
		Components: components,
	}

	w.Header().Set("Content-Type", "application/json")
	if ready {
		w.WriteHeader(http.StatusOK)
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	json.NewEncoder(w).Encode(response)
}

// SimpleChecker creates a simple health checker from a function
func SimpleChecker(name string, checkFunc func() error) Checker {
	return func(ctx context.Context) ComponentHealth {
		if err := checkFunc(); err != nil {
			return ComponentHealth{
				Status:  StatusUnhealthy,
				Message: fmt.Sprintf("%s unhealthy: %v", name, err),
			}
		}
		return ComponentHealth{
			Status:  StatusHealthy,
			Message: fmt.Sprintf("%s is healthy", name),
		}
	}
}

// ContextChecker creates a health checker with context support
func ContextChecker(name string, checkFunc func(ctx context.Context) error) Checker {
	return func(ctx context.Context) ComponentHealth {
		if err := checkFunc(ctx); err != nil {
			return ComponentHealth{
				Status:  StatusUnhealthy,
				Message: fmt.Sprintf("%s unhealthy: %v", name, err),
			}
		}
		return ComponentHealth{
			Status:  StatusHealthy,
			Message: fmt.Sprintf("%s is healthy", name),
		}
	}
}
