package deploy_test

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// resolveScriptPath finds the deploy_local.sh script regardless of whether tests
// are executed from the repository root, examples/camping_app, or examples/camping_app/deploy.
func resolveScriptPath(t *testing.T) string {
	t.Helper()
	candidates := []string{
		"deploy_local.sh",
		"./deploy_local.sh",
		"deploy/deploy_local.sh",
		"./deploy/deploy_local.sh",
		"examples/camping_app/deploy/deploy_local.sh",
		"./examples/camping_app/deploy/deploy_local.sh",
	}

	for _, c := range candidates {
		abs, err := filepath.Abs(c)
		if err == nil {
			if fi, err := os.Stat(abs); err == nil && !fi.IsDir() {
				return abs
			}
		}
	}

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get current working directory: %v", err)
	}

	dir := wd
	for i := 0; i < 5; i++ {
		checkPath := filepath.Join(dir, "deploy_local.sh")
		if fi, err := os.Stat(checkPath); err == nil && !fi.IsDir() {
			return checkPath
		}
		checkPath = filepath.Join(dir, "deploy", "deploy_local.sh")
		if fi, err := os.Stat(checkPath); err == nil && !fi.IsDir() {
			return checkPath
		}
		checkPath = filepath.Join(dir, "examples", "camping_app", "deploy", "deploy_local.sh")
		if fi, err := os.Stat(checkPath); err == nil && !fi.IsDir() {
			return checkPath
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	t.Fatalf("could not locate deploy_local.sh from %s", wd)
	return ""
}

// getFreePort finds an available ephemeral TCP port on localhost.
func getFreePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen on 127.0.0.1:0: %v", err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// TestDeployLocal_Lifecycle exercises the full start -> health -> status -> stop lifecycle
// of the Customer Zero Alpine Escapes local server deployment script.
func TestDeployLocal_Lifecycle(t *testing.T) {
	scriptPath := resolveScriptPath(t)
	scriptDir := filepath.Dir(scriptPath)
	pidFile := filepath.Join(scriptDir, "camping_app.pid")
	logFile := filepath.Join(scriptDir, "camping_app.log")

	// Ensure any prior running instance is terminated
	stopPreCmd := exec.Command(scriptPath, "stop")
	stopPreCmd.Dir = scriptDir
	_ = stopPreCmd.Run()

	port := getFreePort(t)
	portStr := strconv.Itoa(port)

	// Defer cleanup to guarantee zero dangling background processes
	defer func() {
		stopPostCmd := exec.Command(scriptPath, "stop")
		stopPostCmd.Dir = scriptDir
		_ = stopPostCmd.Run()
		_ = os.Remove(pidFile)
		_ = os.Remove(logFile)
	}()

	// 1. Execute ./deploy_local.sh start --port <port>
	startCmd := exec.Command(scriptPath, "start", "--port", portStr)
	startCmd.Dir = scriptDir
	startOut, err := startCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("deploy_local.sh start failed with error: %v\nOutput:\n%s", err, string(startOut))
	}
	if exitCode := startCmd.ProcessState.ExitCode(); exitCode != 0 {
		t.Fatalf("expected exit code 0 for start, got: %d\nOutput:\n%s", exitCode, string(startOut))
	}

	// 2. Query http://127.0.0.1:<port>/health via http.Get
	healthURL := fmt.Sprintf("http://127.0.0.1:%d/health", port)
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(healthURL)
	if err != nil {
		t.Fatalf("HTTP GET %s failed: %v", healthURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected HTTP 200 OK, got: %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed reading health response body: %v", err)
	}
	bodyStr := string(bodyBytes)
	if !strings.Contains(bodyStr, `"status":"HEALTHY"`) {
		t.Fatalf("expected health body to contain '\"status\":\"HEALTHY\"', got: %s", bodyStr)
	}

	// 3. Execute ./deploy_local.sh status --port <port>
	statusCmd := exec.Command(scriptPath, "status", "--port", portStr)
	statusCmd.Dir = scriptDir
	statusOut, err := statusCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("deploy_local.sh status failed with error: %v\nOutput:\n%s", err, string(statusOut))
	}
	if exitCode := statusCmd.ProcessState.ExitCode(); exitCode != 0 {
		t.Fatalf("expected exit code 0 for status, got: %d\nOutput:\n%s", exitCode, string(statusOut))
	}
	if !strings.Contains(string(statusOut), "RUNNING") {
		t.Fatalf("expected status output to contain 'RUNNING', got:\n%s", string(statusOut))
	}

	// 4. Execute ./deploy_local.sh stop
	stopCmd := exec.Command(scriptPath, "stop")
	stopCmd.Dir = scriptDir
	stopOut, err := stopCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("deploy_local.sh stop failed with error: %v\nOutput:\n%s", err, string(stopOut))
	}
	if exitCode := stopCmd.ProcessState.ExitCode(); exitCode != 0 {
		t.Fatalf("expected exit code 0 for stop, got: %d\nOutput:\n%s", exitCode, string(stopOut))
	}

	// 5. Verify PID file is removed
	if _, err := os.Stat(pidFile); !os.IsNotExist(err) {
		t.Fatalf("expected PID file %s to be removed after stop, but it still exists", pidFile)
	}

	// 6. Verify port is no longer listening
	isPortClosed := false
	for i := 0; i < 20; i++ {
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 100*time.Millisecond)
		if err != nil {
			isPortClosed = true
			break
		}
		conn.Close()
		time.Sleep(100 * time.Millisecond)
	}
	if !isPortClosed {
		t.Fatalf("port %d is still listening after deploy_local.sh stop", port)
	}
}

// TestDeployLocal_Help verifies that ./deploy_local.sh --help displays usage descriptions
// for all lifecycle verbs: start, stop, restart, status.
func TestDeployLocal_Help(t *testing.T) {
	scriptPath := resolveScriptPath(t)
	scriptDir := filepath.Dir(scriptPath)

	helpCmd := exec.Command(scriptPath, "--help")
	helpCmd.Dir = scriptDir
	helpOut, err := helpCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("deploy_local.sh --help failed with error: %v\nOutput:\n%s", err, string(helpOut))
	}
	if exitCode := helpCmd.ProcessState.ExitCode(); exitCode != 0 {
		t.Fatalf("expected exit code 0 for --help, got: %d\nOutput:\n%s", exitCode, string(helpOut))
	}

	outStr := string(helpOut)
	requiredKeywords := []string{"start", "stop", "restart", "status"}
	for _, kw := range requiredKeywords {
		if !strings.Contains(outStr, kw) {
			t.Fatalf("expected help output to contain usage description for '%s', got:\n%s", kw, outStr)
		}
	}
}

// TestDeployLocal_Restart verifies graceful restart of the local server daemon.
func TestDeployLocal_Restart(t *testing.T) {
	scriptPath := resolveScriptPath(t)
	scriptDir := filepath.Dir(scriptPath)
	pidFile := filepath.Join(scriptDir, "camping_app.pid")
	logFile := filepath.Join(scriptDir, "camping_app.log")

	// Pre-cleanup
	stopPreCmd := exec.Command(scriptPath, "stop")
	stopPreCmd.Dir = scriptDir
	_ = stopPreCmd.Run()

	port := getFreePort(t)
	portStr := strconv.Itoa(port)

	defer func() {
		stopPostCmd := exec.Command(scriptPath, "stop")
		stopPostCmd.Dir = scriptDir
		_ = stopPostCmd.Run()
		_ = os.Remove(pidFile)
		_ = os.Remove(logFile)
	}()

	// 1. Start server
	startCmd := exec.Command(scriptPath, "start", "--port", portStr)
	startCmd.Dir = scriptDir
	if startOut, err := startCmd.CombinedOutput(); err != nil {
		t.Fatalf("start failed: %v\n%s", err, string(startOut))
	}

	// 2. Restart server
	restartCmd := exec.Command(scriptPath, "restart", "--port", portStr)
	restartCmd.Dir = scriptDir
	if restartOut, err := restartCmd.CombinedOutput(); err != nil {
		t.Fatalf("restart failed: %v\n%s", err, string(restartOut))
	}

	// 3. Verify health endpoint after restart
	healthURL := fmt.Sprintf("http://127.0.0.1:%d/health", port)
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(healthURL)
	if err != nil {
		t.Fatalf("HTTP GET %s failed after restart: %v", healthURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected HTTP 200 after restart, got %d", resp.StatusCode)
	}

	// 4. Stop server
	stopCmd := exec.Command(scriptPath, "stop")
	stopCmd.Dir = scriptDir
	if stopOut, err := stopCmd.CombinedOutput(); err != nil {
		t.Fatalf("stop failed: %v\n%s", err, string(stopOut))
	}

	// Verify PID file is gone
	if _, err := os.Stat(pidFile); !os.IsNotExist(err) {
		t.Fatalf("expected PID file %s to be removed after restart test stop", pidFile)
	}
}
