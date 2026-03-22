package main

import (
	"fmt"
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
	ltProcess               *os.Process
	mu                      sync.Mutex
}

func NewServiceManager(config ServiceConfig) *ServiceManager {
	return &ServiceManager{config: config}
}

func (sm *ServiceManager) StartAll() error {
	if err := sm.StartWhisperServer("base"); err != nil {
		return fmt.Errorf("whisper-server: %w", err)
	}
	if err := sm.StartLibreTranslate(); err != nil {
		return fmt.Errorf("libretranslate: %w", err)
	}
	return nil
}

func (sm *ServiceManager) StopAll() {
	sm.StopWhisperServer()
	sm.StopLibreTranslate()
}

func (sm *ServiceManager) StartWhisperServer(modelSize string) error {
	whisperBinary, whisperModel := resolveWhisperAssets(sm.config.SearchRoots, modelSize)

	sm.mu.Lock()
	currentProcess := sm.whisperProcess
	currentModel := sm.currentWhisperModelPath
	sm.mu.Unlock()

	if currentProcess != nil {
		if currentModel == whisperModel && sm.IsWhisperRunning() {
			return nil
		}
		sm.StopWhisperServer()
	} else if sm.IsWhisperRunning() {
		return nil
	}

	if err := validateWhisperStartup(whisperBinary, whisperModel); err != nil {
		return err
	}

	cmd := exec.Command(
		whisperBinary,
		"-m", whisperModel,
		"--port", fmt.Sprintf("%d", sm.config.WhisperPort),
	)
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start whisper-server %q: %w", whisperBinary, err)
	}

	sm.mu.Lock()
	sm.whisperProcess = cmd.Process
	sm.currentWhisperModelPath = whisperModel
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
		_ = sm.whisperProcess.Kill()
		_, _ = sm.whisperProcess.Wait()
		sm.whisperProcess = nil
	}
	sm.currentWhisperModelPath = ""
}

func (sm *ServiceManager) StartLibreTranslate() error {
	sm.mu.Lock()
	currentProcess := sm.ltProcess
	sm.mu.Unlock()

	if currentProcess != nil {
		if sm.IsLibreTranslateRunning() {
			return nil
		}
		sm.StopLibreTranslate()
	} else if sm.IsLibreTranslateRunning() {
		return nil
	}

	if err := validateCommandAvailability("libretranslate", "libretranslate"); err != nil {
		return err
	}

	cmd := exec.Command(
		"libretranslate",
		"--port", fmt.Sprintf("%d", sm.config.LibreTranslatePort),
	)
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start libretranslate: %w", err)
	}

	sm.mu.Lock()
	sm.ltProcess = cmd.Process
	sm.mu.Unlock()

	if err := waitForService(localServiceURL(sm.config.LibreTranslatePort, "/languages"), 120*time.Second); err != nil {
		sm.StopLibreTranslate()
		return fmt.Errorf("libretranslate failed to start: %w", err)
	}

	return nil
}

func (sm *ServiceManager) StopLibreTranslate() {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.ltProcess != nil {
		_ = sm.ltProcess.Kill()
		_, _ = sm.ltProcess.Wait()
		sm.ltProcess = nil
	}
}

func (sm *ServiceManager) IsWhisperRunning() bool {
	return isServiceHealthy(localServiceURL(sm.config.WhisperPort, "/health"))
}

func (sm *ServiceManager) IsLibreTranslateRunning() bool {
	return isServiceHealthy(localServiceURL(sm.config.LibreTranslatePort, "/languages"))
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

func validateWhisperStartup(binaryPath, modelPath string) error {
	if err := validateCommandAvailability(binaryPath, "whisper-server"); err != nil {
		return err
	}
	if info, err := os.Stat(modelPath); err != nil || info.IsDir() {
		return fmt.Errorf("whisper model not found at %q", modelPath)
	}
	return nil
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

func isPathReference(path string) bool {
	if filepath.IsAbs(path) {
		return true
	}

	return strings.ContainsRune(path, os.PathSeparator) || strings.Contains(path, "/")
}
