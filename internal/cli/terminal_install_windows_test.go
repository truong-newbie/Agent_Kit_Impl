//go:build windows

package cli

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/adapter/codex"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/catalog"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/installer"
)

func buildStubZip(t *testing.T, ver string, corrupt bool, compiledExe []byte) ([]byte, string) {
	t.Helper()
	if corrupt {
		raw := []byte("not-a-valid-zip-archive")
		sum := sha256.Sum256(raw)
		return raw, hex.EncodeToString(sum[:])
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	payload := compiledExe
	if len(payload) == 0 {
		payload = []byte("MZ-stub-" + ver)
	}
	fw, err := zw.Create("dtkit.exe")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(payload); err != nil {
		t.Fatal(err)
	}
	_ = zw.Close()
	sum := sha256.Sum256(buf.Bytes())
	return buf.Bytes(), hex.EncodeToString(sum[:])
}

func buildRealStubExe(t *testing.T, ver string) []byte {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		return nil
	}
	srcDir := t.TempDir()
	mainGo := fmt.Sprintf("package main\nimport \"fmt\"\nfunc main(){fmt.Println(`{\"version\":\"%s\"}`)}\n", ver)
	if err := os.WriteFile(filepath.Join(srcDir, "main.go"), []byte(mainGo), 0644); err != nil {
		t.Fatal(err)
	}
	outExe := filepath.Join(srcDir, "dtkit.exe")
	cmd := exec.Command("go", "build", "-o", outExe, "main.go")
	cmd.Dir = srcDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Logf("stub go build skipped (%v): %s", err, out)
		return nil
	}
	raw, err := os.ReadFile(outExe)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestTerminalInstallScriptWindows(t *testing.T) {
	psPath, err := exec.LookPath("powershell")
	if err != nil {
		t.Skip("powershell not available on PATH")
	}
	scriptPath, err := filepath.Abs(filepath.Join("..", "..", "scripts", "install.ps1"))
	if err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	archives := map[string][]byte{}
	sumsBody := ""

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.URL.Path == "/SHA256SUMS" {
			_, _ = w.Write([]byte(sumsBody))
			return
		}
		name := strings.TrimPrefix(r.URL.Path, "/")
		if data, ok := archives[name]; ok {
			_, _ = w.Write(data)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	binDir := filepath.Join(base, "bin")
	destExe := filepath.Join(binDir, "dtkit.exe")

	exeV1 := buildRealStubExe(t, "0.4.0-rc.1")
	skipExec := len(exeV1) == 0

	runPS := func(ver string, extraEnv ...string) (string, error) {
		args := []string{"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", scriptPath, "-Version", ver, "-BinDir", binDir, "-BaseUrl", srv.URL}
		if skipExec {
			args = append(args, "-SkipBinaryExecCheck")
		}
		cmd := exec.Command(psPath, args...)
		cmd.Env = append(os.Environ(), extraEnv...)
		out, err := cmd.CombinedOutput()
		return string(out), err
	}

	a1Name := "dtkit_0.4.0-rc.1_windows_amd64.zip"
	a1Bytes, a1Hash := buildStubZip(t, "0.4.0-rc.1", false, exeV1)
	mu.Lock()
	archives[a1Name] = a1Bytes
	sumsBody = fmt.Sprintf("%s  %s\n", a1Hash, a1Name)
	mu.Unlock()

	if out, err := runPS("0.4.0-rc.1"); err != nil {
		t.Fatalf("fresh Windows install failed: %v\n%s", err, out)
	}
	v1OnDisk, err := os.ReadFile(destExe)
	if err != nil || len(v1OnDisk) == 0 {
		t.Fatalf("installed dtkit.exe missing: %v", err)
	}

	// Register active Codex hook pointing to destExe.
	proj := filepath.Join(base, "proj")
	home := filepath.Join(base, "home")
	codexHome := filepath.Join(base, "codex-home")
	for _, d := range []string{proj, home, codexHome} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}
	pkg, src, err := catalog.Engineer()
	if err != nil {
		t.Fatal(err)
	}
	arts, _ := codex.Render(pkg, src, "project")
	projRoots := map[string]string{"project": proj}
	globRoots := map[string]string{"user-home": home, "codex-home": codexHome}
	if _, err := installer.FreshInstall("project", projRoots, pkg, src, arts, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, _, err := installer.EnableHook("project", "session-context", projRoots, globRoots, destExe, false, time.Now(), nil); err != nil {
		t.Fatalf("EnableHook failed: %v", err)
	}

	exeV2 := buildRealStubExe(t, "0.4.0-rc.2")
	a2Name := "dtkit_0.4.0-rc.2_windows_amd64.zip"
	a2Bytes, a2Hash := buildStubZip(t, "0.4.0-rc.2", false, exeV2)

	// Negative checks: checksum mismatch, missing entry, duplicate entry, wrong arch, corrupt zip.
	mu.Lock()
	archives[a2Name] = a2Bytes
	sumsBody = fmt.Sprintf("%s  %s\n", strings.Repeat("0", 64), a2Name)
	mu.Unlock()
	if _, err := runPS("0.4.0-rc.2"); err == nil {
		t.Fatal("expected failure on Windows checksum mismatch")
	}
	mu.Lock()
	sumsBody = ""
	mu.Unlock()
	if _, err := runPS("0.4.0-rc.2"); err == nil {
		t.Fatal("expected failure on missing SHA256SUMS entry")
	}
	mu.Lock()
	sumsBody = fmt.Sprintf("%s  %s\n%s  %s\n", a2Hash, a2Name, a2Hash, a2Name)
	mu.Unlock()
	if _, err := runPS("0.4.0-rc.2"); err == nil {
		t.Fatal("expected failure on duplicate SHA256SUMS entry")
	}
	if out, err := runPS("0.4.0-rc.2", "DTKIT_TEST_ARCH=Arm64"); err == nil || !strings.Contains(strings.Join(strings.Fields(out), " "), "Unsupported Windows architecture") {
		t.Fatalf("expected unsupported arch rejection on Windows Arm64, got err=%v out=%s", err, out)
	}

	// Exclusive file lock on destExe prevents replacement and keeps original v1 binary intact.
	mu.Lock()
	sumsBody = fmt.Sprintf("%s  %s\n", a2Hash, a2Name)
	mu.Unlock()
	pPtr, _ := syscall.UTF16PtrFromString(destExe)
	h, lockErr := syscall.CreateFile(pPtr, syscall.GENERIC_READ|syscall.GENERIC_WRITE, 0, nil, syscall.OPEN_EXISTING, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if lockErr != nil {
		t.Fatalf("failed to lock destExe: %v", lockErr)
	}
	lockOut, lockUpErr := runPS("0.4.0-rc.2")
	_ = syscall.CloseHandle(h)
	if lockUpErr == nil {
		t.Fatalf("expected upgrade failure while dtkit.exe is exclusively locked; out=%s", lockOut)
	}
	afterFailBytes, err := os.ReadFile(destExe)
	if err != nil || !bytes.Equal(afterFailBytes, v1OnDisk) {
		t.Fatalf("locked dtkit.exe altered after failed upgrade")
	}

	// Valid upgrade succeeds once unlocked and keeps active hook healthy.
	if out, err := runPS("0.4.0-rc.2"); err != nil {
		t.Fatalf("valid Windows upgrade failed: %v\n%s", err, out)
	}
	insp, err := installer.InspectHooks("project", projRoots)
	if err != nil || len(insp) != 1 || insp[0].Status != "configured_pending_trust" {
		t.Fatalf("hook broken after Windows binary upgrade: %+v err=%v", insp, err)
	}
}
