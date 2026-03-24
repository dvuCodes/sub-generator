package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

var stdoutMu sync.Mutex

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

func main() {
	svcConfig := DefaultServiceConfig()
	svcManager := NewServiceManager(svcConfig)
	pipeline := NewPipeline(svcManager)

	// Graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
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

		handleCommand(cmd, pipeline, svcManager)
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "stdin read error: %v\n", err)
	}

	svcManager.StopAll()
}

func handleCommand(cmd Command, pipeline *Pipeline, svcManager *ServiceManager) {
	switch cmd.Command {
	case "generate":
		pipeline.Run(cmd)

	case "list_languages":
		langs, err := listAvailableLanguages(svcManager)
		if err != nil {
			sendError("Failed to list languages", err.Error())
			return
		}
		sendJSON(LanguagesResponse{
			Type:      "languages",
			Installed: langs,
		})

	case "system_info":
		whisperOK := svcManager.IsWhisperRunning()
		ltOK := svcManager.IsLibreTranslateRunning()
		sendJSON(SystemInfoResponse{
			Type:           "system_info",
			WhisperServer:  whisperOK,
			LibreTranslate: ltOK,
			GPU:            detectGPU(),
		})

	case "start_services":
		if err := svcManager.StartAll(); err != nil {
			sendError("Failed to start services", err.Error())
			return
		}
		sendJSON(map[string]string{"type": "services_started"})

	case "stop_services":
		svcManager.StopAll()
		sendJSON(map[string]string{"type": "services_stopped"})

	default:
		sendError("Unknown command", fmt.Sprintf("command '%s' is not recognized", cmd.Command))
	}
}

func listAvailableLanguages(svcManager libreTranslateService) ([]LanguagePair, error) {
	if !svcManager.IsLibreTranslateRunning() {
		if err := svcManager.StartLibreTranslate(); err != nil {
			return nil, err
		}
	}

	return newLanguageLister(svcManager.LibreTranslatePort()).ListLanguages()
}

func sendJSON(v any) {
	data, err := json.Marshal(v)
	if err != nil {
		fmt.Fprintf(os.Stderr, "JSON marshal error: %v\n", err)
		return
	}
	stdoutMu.Lock()
	fmt.Println(string(data))
	stdoutMu.Unlock()
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

func sendTimerProgress(stage string, percent float64, message string, elapsedSecs float64, etaSecs float64) {
	resp := ProgressResponse{
		Type:        "progress",
		Stage:       stage,
		Percent:     percent,
		Message:     message,
		ElapsedSecs: &elapsedSecs,
	}
	if etaSecs > 0 {
		resp.ETASecs = &etaSecs
	}
	sendJSON(resp)
}

func detectGPU() string {
	// Try nvidia-smi to detect GPU
	// This is a simple check - just return the GPU name or "none"
	out, err := runCommand("nvidia-smi", "--query-gpu=name", "--format=csv,noheader,nounits")
	if err != nil {
		return "none"
	}
	return out
}
