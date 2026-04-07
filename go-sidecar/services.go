package main

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const loopbackHost = "127.0.0.1"

type ServiceManager struct {
	config                  ServiceConfig
	whisperProcess          *os.Process
	currentWhisperModelPath string
	currentVADModelPath     string
	llamaProcess            *os.Process
	currentLlamaModelPath   string
	mlBackendProcess        *os.Process
	mu                      sync.Mutex
}

func NewServiceManager(config ServiceConfig) *ServiceManager {
	return &ServiceManager{config: config}
}

func (sm *ServiceManager) StartAll() error {
	if err := sm.StartWhisperServer("base"); err != nil {
		return fmt.Errorf("whisper-server: %w", err)
	}
	if err := sm.StartLlamaServer(); err != nil {
		return fmt.Errorf("llama-server: %w", err)
	}
	if err := sm.StartMLBackend(); err != nil {
		return fmt.Errorf("ml-backend: %w", err)
	}
	return nil
}

func (sm *ServiceManager) StopAll() {
	sm.StopWhisperServer()
	sm.StopLlamaServer()
	sm.StopMLBackend()
}

func (sm *ServiceManager) StartWhisperServer(modelSize string) error {
	whisperBinary, whisperModel := resolveWhisperAssets(sm.config.SearchRoots, modelSize)
	vadModel := resolveVADModelPath(sm.config.SearchRoots)

	sm.mu.Lock()
	currentProcess := sm.whisperProcess
	currentModel := sm.currentWhisperModelPath
	currentVAD := sm.currentVADModelPath
	sm.mu.Unlock()

	healthy := sm.IsWhisperRunning()
	if err := rejectUnmanagedHealthyService("whisper-server", sm.config.WhisperPort, currentProcess, healthy); err != nil {
		return err
	}

	if currentProcess != nil {
		if currentModel == whisperModel && currentVAD == vadModel && healthy {
			return nil
		}
		sm.StopWhisperServer()
	}

	if err := validateWhisperStartup(sm.config.SearchRoots, whisperBinary, whisperModel); err != nil {
		return err
	}

	if err := validateWhisperRuntimeDependencies(sm.config.SearchRoots, whisperBinary, whisperModel, true); err != nil {
		return err
	}

	if vadModel == "" {
		fmt.Fprintf(os.Stderr, "warning: VAD model not found in search roots; whisper-server will start without VAD support\n")
	}

	cmd := buildWhisperCommand(whisperBinary, whisperModel, vadModel, sm.config.WhisperPort)
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return normalizeWhisperStartupError(
			sm.config.SearchRoots,
			whisperBinary,
			whisperModel,
			fmt.Errorf("failed to start whisper-server %q: %w", whisperBinary, err),
		)
	}
	assignToJob(cmd.Process)

	sm.mu.Lock()
	sm.whisperProcess = cmd.Process
	sm.currentWhisperModelPath = whisperModel
	sm.currentVADModelPath = vadModel
	sm.config.WhisperServerPath = whisperBinary
	sm.config.WhisperModelPath = whisperModel
	sm.mu.Unlock()

	if err := waitForService(localServiceURL(sm.config.WhisperPort, "/health"), 30*time.Second); err != nil {
		sm.StopWhisperServer()
		return fmt.Errorf("whisper-server failed to start using %q with model %q: %w", whisperBinary, whisperModel, err)
	}

	return nil
}

func (sm *ServiceManager) StopWhisperServer() {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.stopWhisperServerLocked()
}

func (sm *ServiceManager) stopWhisperServerLocked() {
	if sm.whisperProcess != nil {
		if err := sm.whisperProcess.Kill(); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to kill whisper-server process: %v\n", err)
		}
		_, _ = sm.whisperProcess.Wait()
		sm.whisperProcess = nil
	}
	sm.currentWhisperModelPath = ""
	sm.currentVADModelPath = ""
}

func (sm *ServiceManager) StartLlamaServer() error {
	llamaBinary := resolveLlamaServerBinary(sm.config.SearchRoots)
	gemmaModel := resolveGemmaModelPath(sm.config.SearchRoots)

	sm.mu.Lock()
	currentProcess := sm.llamaProcess
	currentModel := sm.currentLlamaModelPath
	sm.mu.Unlock()

	healthy := sm.IsLlamaServerRunning()
	if err := rejectUnmanagedHealthyService("llama-server", sm.config.LlamaServerPort, currentProcess, healthy); err != nil {
		return err
	}

	if shouldReuseManagedLlamaProcess(currentProcess, currentModel, gemmaModel, healthy) {
		return nil
	}
	if currentProcess != nil {
		sm.StopLlamaServer()
	}

	if err := validateCommandAvailability(llamaBinary, "llama-server"); err != nil {
		return fmt.Errorf(
			"llama-server is required for translation. Download it from llama.cpp releases and place it in services/llama-server/: %w",
			err,
		)
	}

	if gemmaModel == "" {
		return fmt.Errorf(
			"GemmaTranslate model not found. It will be downloaded automatically when translation is requested, or place %s in services/llama-server/models/",
			gemmaModelFilenameConst,
		)
	}

	cmd := buildLlamaCommand(llamaBinary, gemmaModel, sm.config.LlamaServerPort)
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start llama-server %q: %w", llamaBinary, err)
	}
	assignToJob(cmd.Process)

	sm.mu.Lock()
	sm.llamaProcess = cmd.Process
	sm.currentLlamaModelPath = gemmaModel
	sm.mu.Unlock()

	if err := waitForService(localServiceURL(sm.config.LlamaServerPort, "/health"), 120*time.Second); err != nil {
		sm.StopLlamaServer()
		return fmt.Errorf("llama-server failed to start using %q with model %q: %w", llamaBinary, gemmaModel, err)
	}

	return nil
}

func (sm *ServiceManager) StopLlamaServer() {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.llamaProcess != nil {
		_ = sm.llamaProcess.Kill()
		_, _ = sm.llamaProcess.Wait()
		sm.llamaProcess = nil
	}
	sm.currentLlamaModelPath = ""
}

func (sm *ServiceManager) StartMLBackend() error {
	launcherPath := resolveMLBackendLauncher(sm.config.SearchRoots)
	pythonPath := resolveMLBackendPython(sm.config.SearchRoots)
	scriptPath := resolveMLBackendScriptPath(sm.config.SearchRoots)

	if scriptPath == "" && !fileExists(launcherPath) {
		return fmt.Errorf(
			"ml-backend setup is incomplete: launcher or service.py missing under %q",
			preferredMLBackendInstallDir(sm.config.SearchRoots),
		)
	}
	if !fileExists(launcherPath) {
		if err := validateCommandAvailability(pythonPath, "python"); err != nil {
			return fmt.Errorf("python runtime is required for ml-backend: %w", err)
		}
	}

	sm.mu.Lock()
	currentProcess := sm.mlBackendProcess
	sm.mu.Unlock()

	healthy := sm.IsMLBackendRunning()
	if err := rejectUnmanagedHealthyService("ml-backend", sm.config.MLBackendPort, currentProcess, healthy); err != nil {
		return err
	}
	if err := rejectUnmanagedListeningService("ml-backend", sm.config.MLBackendPort, currentProcess, healthy); err != nil {
		return err
	}
	if currentProcess != nil && healthy {
		return nil
	}
	if currentProcess != nil {
		sm.StopMLBackend()
	}

	cmd, err := buildMLBackendCommand(launcherPath, pythonPath, scriptPath, sm.config.MLBackendPort)
	if err != nil {
		return fmt.Errorf("python runtime is required for ml-backend: %w", err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start ml-backend: %w", err)
	}
	assignToJob(cmd.Process)

	sm.mu.Lock()
	sm.mlBackendProcess = cmd.Process
	sm.mu.Unlock()

	if err := waitForService(localServiceURL(sm.config.MLBackendPort, "/health"), 45*time.Second); err != nil {
		sm.StopMLBackend()
		return fmt.Errorf("ml-backend failed to start: %w", err)
	}

	return nil
}

func (sm *ServiceManager) StopMLBackend() {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.mlBackendProcess != nil {
		_ = terminateManagedProcess(sm.mlBackendProcess)
		_, _ = sm.mlBackendProcess.Wait()
		sm.mlBackendProcess = nil
	}
}

func (sm *ServiceManager) IsWhisperRunning() bool {
	return isServiceHealthy(localServiceURL(sm.config.WhisperPort, "/health"))
}

func (sm *ServiceManager) IsLlamaServerRunning() bool {
	return isServiceHealthy(localServiceURL(sm.config.LlamaServerPort, "/health"))
}

func (sm *ServiceManager) IsMLBackendRunning() bool {
	return isServiceHealthy(localServiceURL(sm.config.MLBackendPort, "/health"))
}

func (sm *ServiceManager) LlamaServerPort() int {
	return sm.config.LlamaServerPort
}

func (sm *ServiceManager) MLBackendURL() string {
	return localServiceBaseURL(sm.config.MLBackendPort)
}

func isServiceHealthy(url string) bool {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func waitForService(url string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if isServiceHealthy(url) {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("service at %s did not become healthy within %s", url, timeout)
}

func localServiceBaseURL(port int) string {
	return fmt.Sprintf("http://%s:%d", loopbackHost, port)
}

func localServiceURL(port int, path string) string {
	return localServiceBaseURL(port) + path
}

func runCommand(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func buildWhisperCommand(binaryPath, modelPath, vadModelPath string, port int) *exec.Cmd {
	args := []string{
		"-m", modelPath,
		"--port", fmt.Sprintf("%d", port),
		"--convert",
	}
	if detectGPU() == "none" {
		args = append(args, "--no-flash-attn")
	}
	if vadModelPath != "" {
		args = append(args, "--vad-model", vadModelPath)
	}
	return exec.Command(binaryPath, args...)
}

func buildLlamaCommand(binaryPath, modelPath string, port int) *exec.Cmd {
	gpuLayers := "999"
	ctxSize := "2048"
	if detectGPU() == "none" {
		gpuLayers = "0"
		ctxSize = "1024"
	}
	args := []string{
		"-m", modelPath,
		"--port", fmt.Sprintf("%d", port),
		"--host", loopbackHost,
		"--n-gpu-layers", gpuLayers,
		"--ctx-size", ctxSize,
	}
	cmd := exec.Command(binaryPath, args...)
	cmd.Dir = filepath.Dir(binaryPath)
	return cmd
}

func buildMLBackendCommand(launcherPath, pythonPath, scriptPath string, port int) (*exec.Cmd, error) {
	if scriptPath == "" {
		if fileExists(launcherPath) {
			cmd := exec.Command(launcherPath, "--host", loopbackHost, "--port", fmt.Sprintf("%d", port))
			cmd.Dir = filepath.Dir(launcherPath)
			return cmd, nil
		}
		return nil, fmt.Errorf("ml-backend service.py missing")
	}

	args := []string{scriptPath, "--host", loopbackHost, "--port", fmt.Sprintf("%d", port)}
	cmd := exec.Command(pythonPath, args...)
	cmd.Dir = filepath.Dir(scriptPath)
	return cmd, nil
}

func fileExists(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func terminateManagedProcess(process *os.Process) error {
	if process == nil {
		return nil
	}
	if os.PathSeparator == '\\' {
		return exec.Command("taskkill", "/T", "/F", "/PID", fmt.Sprintf("%d", process.Pid)).Run()
	}
	return process.Kill()
}

func shouldReuseManagedLlamaProcess(currentProcess *os.Process, currentModel string, expectedModel string, healthy bool) bool {
	return currentProcess != nil && currentModel == expectedModel && healthy
}

func rejectUnmanagedHealthyService(serviceName string, port int, currentProcess *os.Process, healthy bool) error {
	if currentProcess != nil || !healthy {
		return nil
	}

	return fmt.Errorf(
		"%s is already responding on port %d but is not managed by SubGen. Startup was aborted to avoid reusing stale settings; stop the existing %s process and retry",
		serviceName,
		port,
		serviceName,
	)
}

func rejectUnmanagedListeningService(serviceName string, port int, currentProcess *os.Process, healthy bool) error {
	if currentProcess != nil || healthy || !isPortListening(port) {
		return nil
	}

	return fmt.Errorf(
		"%s is already listening on port %d but is not managed by SubGen. Startup was aborted to avoid conflicting with another local service; stop the existing process using port %d and retry",
		serviceName,
		port,
		port,
	)
}

func isPortListening(port int) bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", loopbackHost, port), 500*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func validateWhisperStartup(searchRoots []string, binaryPath, modelPath string) error {
	missingBinary := false
	if err := validateCommandAvailability(binaryPath, "whisper-server"); err != nil {
		missingBinary = true
	}

	missingModel := false
	if info, err := os.Stat(modelPath); err != nil || info.IsDir() {
		missingModel = true
	}

	if missingBinary || missingModel {
		return missingWhisperSetupError(searchRoots, binaryPath, modelPath, missingBinary, missingModel)
	}

	return nil
}

func validateWhisperRuntimeDependencies(searchRoots []string, binaryPath, modelPath string, convertInput bool) error {
	if !convertInput {
		return nil
	}

	if err := validateCommandAvailability("ffmpeg", "ffmpeg"); err != nil {
		return fmt.Errorf(
			"ffmpeg is required to transcribe video files with whisper-server --convert, but %w. Install ffmpeg and add it to PATH before generating subtitles",
			err,
		)
	}

	return nil
}

func missingWhisperSetupError(searchRoots []string, binaryPath, modelPath string, missingBinary, missingModel bool) error {
	installDir := preferredWhisperInstallDir(searchRoots)
	expectedBinary := preferredWhisperBinaryPath(installDir)
	expectedModel := filepath.Join(installDir, "models", filepath.Base(modelPath))
	binaryLocation := expectedBinary
	modelLocation := expectedModel

	if isPathReference(binaryPath) {
		binaryLocation = binaryPath
	}
	if isPathReference(modelPath) {
		modelLocation = modelPath
	}

	problems := make([]string, 0, 2)
	if missingBinary {
		problems = append(problems, fmt.Sprintf("binary missing at %q", binaryLocation))
	}
	if missingModel {
		problems = append(problems, fmt.Sprintf("model missing at %q", modelLocation))
	}

	message := fmt.Sprintf(
		"whisper-server setup is incomplete: %s. Install whisper.cpp's whisper-server and GGML models under %q (see %q), or make whisper-server available on PATH",
		strings.Join(problems, "; "),
		installDir,
		filepath.Join(installDir, "README.md"),
	)

	if !missingBinary && isPathReference(binaryPath) {
		return fmt.Errorf("%s. Configured binary path: %q", message, binaryPath)
	}

	return fmt.Errorf("%s.", message)
}

func preferredWhisperBinaryPath(installDir string) string {
	for _, executableName := range whisperExecutableCandidates() {
		candidate := filepath.Join(installDir, executableName)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}

	return filepath.Join(installDir, whisperExecutableName())
}

func normalizeWhisperStartupError(searchRoots []string, binaryPath, modelPath string, err error) error {
	if err == nil {
		return nil
	}
	if !isMissingWhisperExecutableError(err.Error()) {
		return err
	}

	missingModel := false
	if info, statErr := os.Stat(modelPath); statErr != nil || info.IsDir() {
		missingModel = true
	}

	return missingWhisperSetupError(searchRoots, binaryPath, modelPath, true, missingModel)
}

func preferredWhisperInstallDir(searchRoots []string) string {
	for _, root := range normalizeSearchRoots(searchRoots) {
		candidate := filepath.Join(root, "services", "whisper-server")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
	}

	if len(searchRoots) > 0 {
		return filepath.Join(searchRoots[0], "services", "whisper-server")
	}

	return filepath.Join("services", "whisper-server")
}

func whisperExecutableName() string {
	if os.PathSeparator == '\\' {
		return "whisper-server.exe"
	}
	return "whisper-server"
}

func validateCommandAvailability(commandPath, displayName string) error {
	if isPathReference(commandPath) {
		if info, err := os.Stat(commandPath); err != nil || info.IsDir() {
			return fmt.Errorf("%s executable not found at %q", displayName, commandPath)
		}
		return nil
	}

	if _, err := exec.LookPath(commandPath); err != nil {
		return fmt.Errorf("%s executable %q not found in PATH", displayName, commandPath)
	}

	return nil
}

func isMissingWhisperExecutableError(message string) bool {
	normalized := strings.ToLower(message)
	if !strings.Contains(normalized, "whisper-server") {
		return false
	}

	return strings.Contains(normalized, "not found in path") ||
		strings.Contains(normalized, "executable file not found")
}

func isPathReference(path string) bool {
	if filepath.IsAbs(path) {
		return true
	}

	return strings.ContainsRune(path, os.PathSeparator) || strings.Contains(path, "/")
}
