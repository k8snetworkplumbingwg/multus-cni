package api

import (
	"errors"
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

	server := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			pathCh <- r.URL.Path
			w.WriteHeader(http.StatusOK)
		}),
	}

	serveErrCh := make(chan error, 1)
	go func() {
		serveErrCh <- server.Serve(listener)
	}()

	t.Cleanup(func() {
		if err := server.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			t.Errorf("failed to close test server: %v", err)
		}
		serveErr := <-serveErrCh
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			t.Errorf("test server Serve() failed: %v", serveErr)
		}
		if err := listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			t.Errorf("failed to close test listener: %v", err)
		}
	})

	select {
	case serveErr := <-serveErrCh:
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			t.Fatalf("test server Serve() failed during startup: %v", serveErr)
		}
	default:
	}

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
