package api

import (
	"net"
	"net/http"
	"testing"
)

func startTestUnixSocketServer(t *testing.T) (string, <-chan string) {
	t.Helper()

	tmpDir := t.TempDir()
	pathCh := make(chan string, 1)

	listener, err := net.Listen("unix", SocketPath(tmpDir))
	if err != nil {
		t.Fatalf("failed to listen on unix socket: %v", err)
	}
	t.Cleanup(func() {
		_ = listener.Close()
	})

	server := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			pathCh <- r.URL.Path
			w.WriteHeader(http.StatusOK)
		}),
	}
	t.Cleanup(func() {
		_ = server.Close()
	})

	go func() {
		_ = server.Serve(listener)
	}()

	return tmpDir, pathCh
}

func TestWaitUntilAPIReadyUsesReadyzEndpoint(t *testing.T) {
	t.Parallel()

	tmpDir, pathCh := startTestUnixSocketServer(t)

	if err := WaitUntilAPIReady(tmpDir); err != nil {
		t.Fatalf("WaitUntilAPIReady returned error: %v", err)
	}

	gotPath := <-pathCh
	if gotPath != MultusReadyAPIEndpoint {
		t.Fatalf("expected readiness endpoint %q, got %q", MultusReadyAPIEndpoint, gotPath)
	}
}

func TestCheckAPIReadyNowUsesHealthzEndpoint(t *testing.T) {
	t.Parallel()

	tmpDir, pathCh := startTestUnixSocketServer(t)

	if err := CheckAPIReadyNow(tmpDir); err != nil {
		t.Fatalf("CheckAPIReadyNow returned error: %v", err)
	}

	gotPath := <-pathCh
	if gotPath != MultusHealthAPIEndpoint {
		t.Fatalf("expected health endpoint %q, got %q", MultusHealthAPIEndpoint, gotPath)
	}
}
