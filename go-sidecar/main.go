package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
)

var stdoutMu sync.Mutex

type translationService interface {
	IsLlamaServerRunning() bool
	StartLlamaServer() error
	LlamaServerPort() int
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
		langs, err := listAvailableLanguages()
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
		llamaOK := svcManager.IsLlamaServerRunning()
		sendJSON(SystemInfoResponse{
			Type:              "system_info",
			WhisperServer:     whisperOK,
			TranslationEngine: llamaOK,
			GPU:               detectGPU(),
		})

	case "vram_info":
		sendJSON(VramInfoResponse{
			Type: "vram_info",
			Vram: detectVRAM(),
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

func listAvailableLanguages() ([]LanguagePair, error) {
	return StaticLanguagePairs(), nil
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

func detectVRAM() *VRAMInfo {
	out, err := runCommand("nvidia-smi", "--query-gpu=memory.total,memory.used,memory.free", "--format=csv,noheader,nounits")
	if err != nil {
		return nil
	}
	parts := strings.SplitN(strings.TrimSpace(out), ", ", 3)
	if len(parts) != 3 {
		return nil
	}
	total, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	used, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	free, err3 := strconv.Atoi(strings.TrimSpace(parts[2]))
	if err1 != nil || err2 != nil || err3 != nil {
		return nil
	}
	return &VRAMInfo{TotalMiB: total, UsedMiB: used, FreeMiB: free}
}
