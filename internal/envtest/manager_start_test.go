//go:build envtest

package envtest_test

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	ncmanager "github.com/forest/namespace-class/internal/manager"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

func TestManagerExposesHealthAndReadyEndpoints(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	testEnv := &envtest.Environment{
		CRDDirectoryPaths: []string{filepath.Join(repoRoot(t), "config", "crd", "bases")},
	}

	restConfig, err := testEnv.Start()
	if err != nil {
		t.Fatalf("start envtest: %v", err)
	}
	defer func() {
		if err := testEnv.Stop(); err != nil {
			t.Fatalf("stop envtest: %v", err)
		}
	}()

	probeAddress := freeLocalAddress(t)
	mgr, err := ncmanager.New(restConfig, ncmanager.Options{
		MetricsBindAddress:     "0",
		HealthProbeBindAddress: probeAddress,
		LeaderElection:         false,
	})
	if err != nil {
		t.Fatalf("create manager: %v", err)
	}

	managerCtx, stopManager := context.WithCancel(ctx)
	defer stopManager()

	errCh := make(chan error, 1)
	go func() {
		errCh <- mgr.Start(managerCtx)
	}()
	defer func() {
		stopManager()
		select {
		case err := <-errCh:
			if err != nil {
				t.Fatalf("manager returned error: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("manager did not stop")
		}
	}()

	waitForHTTP200(t, fmt.Sprintf("http://%s/healthz", probeAddress))
	waitForHTTP200(t, fmt.Sprintf("http://%s/readyz", probeAddress))
}

func freeLocalAddress(t *testing.T) string {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen on free local port: %v", err)
	}
	defer listener.Close()

	return listener.Addr().String()
}

func waitForHTTP200(t *testing.T, url string) {
	t.Helper()

	deadline := time.Now().Add(10 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
			lastErr = fmt.Errorf("status %s", resp.Status)
		} else {
			lastErr = err
		}
		time.Sleep(100 * time.Millisecond)
	}

	t.Fatalf("%s did not return HTTP 200: %v", url, lastErr)
}
