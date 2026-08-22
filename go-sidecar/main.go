package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
)

type libreTranslateService interface {
	IsLibreTranslateRunning() bool
	StartLibreTranslate() error
	LibreTranslatePort() int
}

type languageLister interface {
	ListLanguages() ([]LanguagePair, error)
}

var newLanguageLister = func(port int) languageLister {
	return NewTranslator(port)
}

// jobManager tracks the single active generate job so it can be cancelled.
type jobManager struct {
	mu     sync.Mutex
	cancel context.CancelFunc
	active bool
}

func (j *jobManager) start(fn func(ctx context.Context)) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.active {
		return fmt.Errorf("another generation job is already running - cancel it first")
	}
	ctx, cancel := context.WithCancel(context.Background())
	j.cancel = cancel
	j.active = true

	go func() {
		defer func() {
			j.mu.Lock()
			j.active = false
			j.cancel = nil
			j.mu.Unlock()
		}()
		fn(ctx)
	}()
	return nil
}

func (j *jobManager) cancelActive() bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	if !j.active || j.cancel == nil {
		return false
	}
	j.cancel()
	return true
}

func (j *jobManager) isActive() bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.active
}

func main() {
	svcConfig := DefaultServiceConfig()
	svcManager := NewServiceManager(svcConfig)
	pipeline := NewPipeline(svcManager)
	jobs := &jobManager{}

	// Graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		if jobs.isActive() {
			jobs.cancelActive()
		}
		svcManager.StopAll()
		os.Exit(0)
	}()

	scanner := bufio.NewScanner(os.Stdin)
	// Increase buffer size for large commands
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var cmd Command
		if err := json.Unmarshal([]byte(line), &cmd); err != nil {
			sendError("Invalid JSON command", err.Error())
			continue
		}

		switch cmd.Command {
		case "generate":
			job := cmd
			if err := jobs.start(func(ctx context.Context) {
				pipeline.Run(ctx, job)
			}); err != nil {
				sendError("Job not started", err.Error())
			}

		case "cancel":
			if !jobs.cancelActive() {
				sendError("Nothing to cancel", "No generation job is currently running")
				continue
			}
			// The running pipeline reports its own CancelledResponse.

		case "list_languages":
			langs, err := listAvailableLanguages(svcManager)
			if err != nil {
				sendError("Failed to list languages", err.Error())
				continue
			}
			sendJSON(LanguagesResponse{
				Type:      "languages",
				Installed: langs,
			})

		case "system_info":
			ffmpegOK := false
			if _, err := LookupFFmpeg(); err == nil {
				ffmpegOK = true
			}
			sendJSON(SystemInfoResponse{
				Type:           "system_info",
				WhisperServer:  svcManager.IsWhisperRunning(),
				LibreTranslate: svcManager.IsLibreTranslateRunning(),
				GPU:            detectGPU(),
				FFmpeg:         ffmpegOK,
				VADModel:       svcManager.HasVADModel(),
			})

		case "start_services":
			if err := svcManager.StartAll(); err != nil {
				sendError("Failed to start services", err.Error())
				continue
			}
			sendJSON(map[string]string{"type": "services_started"})

		case "stop_services":
			svcManager.StopAll()
			sendJSON(map[string]string{"type": "services_stopped"})

		default:
			sendError("Unknown command", fmt.Sprintf("command '%s' is not recognized", cmd.Command))
		}
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "stdin read error: %v\n", err)
	}

	svcManager.StopAll()
}

func listAvailableLanguages(svcManager libreTranslateService) ([]LanguagePair, error) {
	if !svcManager.IsLibreTranslateRunning() {
		if err := svcManager.StartLibreTranslate(); err != nil {
			if isMissingExecutableError(err) {
				return []LanguagePair{}, nil
			}
			return nil, err
		}
	}

	return newLanguageLister(svcManager.LibreTranslatePort()).ListLanguages()
}

func isMissingExecutableError(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "executable") &&
		strings.Contains(message, "not found")
}

// stdoutMu serializes IPC writes: progress heartbeats run concurrently with
// the stdin command loop (e.g. system_info while generating).
var stdoutMu sync.Mutex

func sendJSON(v any) {
	data, err := json.Marshal(v)
	if err != nil {
		fmt.Fprintf(os.Stderr, "JSON marshal error: %v\n", err)
		return
	}
	stdoutMu.Lock()
	defer stdoutMu.Unlock()
	fmt.Println(string(data))
}

func sendError(message, details string) {
	sendJSON(ErrorResponse{
		Type:    "error",
		Message: message,
		Details: details,
	})
}

func sendProgress(stage string, percent float64, message string) {
	sendJSON(ProgressResponse{
		Type:    "progress",
		Stage:   stage,
		Percent: percent,
		Message: message,
	})
}

func sendStage(stage, message string) {
	sendJSON(StageResponse{
		Type:    "stage",
		Stage:   stage,
		Message: message,
	})
}

func sendCancelled() {
	sendJSON(CancelledResponse{
		Type:    "cancelled",
		Message: "Generation cancelled",
	})
}

func detectGPU() string {
	out, err := runCommand("nvidia-smi", "--query-gpu=name", "--format=csv,noheader,nounits")
	if err != nil {
		return "none"
	}
	return out
}
