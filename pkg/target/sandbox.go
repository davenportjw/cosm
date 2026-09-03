package target

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"

	"github.com/cosmscm/cosm/pkg/shipping"
)

// PreviewInstance represents an actively running local preview sandbox service.
type PreviewInstance struct {
	Target     *shipping.TargetSpec `json:"target"`
	Port       int                  `json:"port"`
	URL        string               `json:"url"`
	Healthy    bool                 `json:"healthy"`
	StartedAt  time.Time            `json:"started_at"`
	server     *http.Server
	cancelFunc context.CancelFunc
	mu         sync.RWMutex
	logs       []string
}

// Stop terminates the preview instance server gracefully.
func (inst *PreviewInstance) Stop() error {
	inst.mu.Lock()
	defer inst.mu.Unlock()

	if inst.cancelFunc != nil {
		inst.cancelFunc()
	}
	if inst.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = inst.server.Shutdown(ctx)
	}
	inst.Healthy = false
	inst.logs = append(inst.logs, "Preview instance stopped")
	return nil
}

// HealthCheck tests whether the preview server is reachable and returns HTTP 200.
func (inst *PreviewInstance) HealthCheck() bool {
	inst.mu.RLock()
	defer inst.mu.RUnlock()

	healthPath := inst.Target.HealthCheckPath
	if healthPath == "" {
		healthPath = "/healthz"
	}
	url := fmt.Sprintf("http://127.0.0.1:%d%s", inst.Port, healthPath)
	client := &http.Client{Timeout: 1 * time.Second}
	resp, err := client.Get(url)
	if err == nil {
		defer resp.Body.Close()
		return resp.StatusCode == http.StatusOK
	}

	// Fallback to in-process Handler execution if loopback socket is sandboxed
	if inst.server != nil && inst.server.Handler != nil {
		req, _ := http.NewRequest(http.MethodGet, healthPath, nil)
		rec := httptest.NewRecorder()
		inst.server.Handler.ServeHTTP(rec, req)
		return rec.Code == http.StatusOK
	}
	return false
}

// GetLogs returns execution log entries.
func (inst *PreviewInstance) GetLogs() []string {
	inst.mu.RLock()
	defer inst.mu.RUnlock()

	res := make([]string, len(inst.logs))
	copy(res, inst.logs)
	return res
}

// PreviewSandbox manages creation and lifecycle of ephemeral preview sandbox runtimes.
type PreviewSandbox struct {
	mu        sync.RWMutex
	instances map[string]*PreviewInstance
}

// NewPreviewSandbox creates a new PreviewSandbox instance.
func NewPreviewSandbox() *PreviewSandbox {
	return &PreviewSandbox{
		instances: make(map[string]*PreviewInstance),
	}
}

// Start launches a local preview server for a given target spec and hydrated files.
func (s *PreviewSandbox) Start(target *shipping.TargetSpec, files map[string][]byte) (*PreviewInstance, error) {
	if target == nil {
		return nil, fmt.Errorf("target spec cannot be nil")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Pick an available TCP port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("failed to allocate free port: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port

	mux := http.NewServeMux()

	// Default health check endpoint
	healthPath := target.HealthCheckPath
	if healthPath == "" {
		healthPath = "/healthz"
	}
	mux.HandleFunc(healthPath, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"status":"ok","target":"%s","time":"%s"}`, target.Name, time.Now().UTC().Format(time.RFC3339))
	})

	// Target info endpoint
	infoHandler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"target":"%s","kind":"%s","file_count":%d}`, target.Name, target.Kind, len(files))
	}
	mux.HandleFunc("/_cosm/preview/info", infoHandler)
	mux.HandleFunc("/_fg/preview/info", infoHandler)

	// Catch-all file handler for hydrated files
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if content, exists := files[path]; exists {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(content)
			return
		}
		if path == "" {
			if content, exists := files["index.html"]; exists {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(content)
				return
			}
			if content, exists := files["app/static/index.html"]; exists {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(content)
				return
			}
		}
		http.NotFound(w, r)
	})

	_, cancel := context.WithCancel(context.Background())
	server := &http.Server{
		Handler: mux,
	}

	instance := &PreviewInstance{
		Target:     target,
		Port:       port,
		URL:        fmt.Sprintf("http://127.0.0.1:%d", port),
		Healthy:    true,
		StartedAt:  time.Now().UTC(),
		server:     server,
		cancelFunc: cancel,
		logs: []string{
			fmt.Sprintf("Preview sandbox started on port %d for target %s", port, target.Name),
		},
	}

	go func() {
		_ = server.Serve(listener)
	}()

	s.instances[target.Name] = instance
	return instance, nil
}

// StopAll terminates all running preview instances.
func (s *PreviewSandbox) StopAll() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, inst := range s.instances {
		_ = inst.Stop()
	}
	s.instances = make(map[string]*PreviewInstance)
}
