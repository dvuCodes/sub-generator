package main

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type ServiceManager struct {
	config         ServiceConfig
	whisperProcess *os.Process
	ltProcess      *os.Process
	mu             sync.Mutex
}

func NewServiceManager(config ServiceConfig) *ServiceManager {
	return &ServiceManager{config: config}
}

func (sm *ServiceManager) StartAll() error {
	if err := sm.StartWhisperServer(); err != nil {
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

func (sm *ServiceManager) StartWhisperServer() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.whisperProcess != nil {
		return nil // already running
	}

	// Check if already running externally
	if sm.IsWhisperRunning() {
		return nil
	}

	cmd := exec.Command(
		sm.config.WhisperServerPath,
		"-m", sm.config.WhisperModelPath,
		"--port", fmt.Sprintf("%d", sm.config.WhisperPort),
	)
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start whisper-server: %w", err)
	}

	sm.whisperProcess = cmd.Process

	// Wait for server to be ready
	if err := waitForService(fmt.Sprintf("http://localhost:%d/health", sm.config.WhisperPort), 30*time.Second); err != nil {
		sm.StopWhisperServer()
		return fmt.Errorf("whisper-server failed to start: %w", err)
	}

	return nil
}

func (sm *ServiceManager) StopWhisperServer() {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.whisperProcess != nil {
		_ = sm.whisperProcess.Kill()
		_, _ = sm.whisperProcess.Wait()
		sm.whisperProcess = nil
	}
}

func (sm *ServiceManager) StartLibreTranslate() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.ltProcess != nil {
		return nil // already running
	}

	// Check if already running externally
	if sm.IsLibreTranslateRunning() {
		return nil
	}

	cmd := exec.Command(
		"libretranslate",
		"--port", fmt.Sprintf("%d", sm.config.LibreTranslatePort),
	)
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start libretranslate: %w", err)
	}

	sm.ltProcess = cmd.Process

	// LibreTranslate can take a while to load models
	if err := waitForService(fmt.Sprintf("http://localhost:%d/languages", sm.config.LibreTranslatePort), 120*time.Second); err != nil {
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
	return isServiceHealthy(fmt.Sprintf("http://localhost:%d/health", sm.config.WhisperPort))
}

func (sm *ServiceManager) IsLibreTranslateRunning() bool {
	return isServiceHealthy(fmt.Sprintf("http://localhost:%d/languages", sm.config.LibreTranslatePort))
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

func runCommand(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
