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
var pipelineMu sync.Mutex
var actionRegistry = NewActionRegistry()

var runGenerate = func(pipeline *Pipeline, cmd Command) {
	if !pipelineMu.TryLock() {
		sendError("Generate failed", "Cannot generate while installing. Wait for installation to complete.")
		return
	}
	defer pipelineMu.Unlock()
	pipeline.Run(cmd)
}

type translationService interface {
	IsLlamaServerRunning() bool
	StartLlamaServer() error
	LlamaServerPort() int
}

func main() {
	initJobObject()

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
		go runGenerate(pipeline, cmd)

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

	case "capabilities":
		sendJSON(buildCapabilitiesResponse(svcManager))

	case "system_info":
		whisperOK := svcManager.IsWhisperRunning()
		llamaOK := svcManager.IsLlamaServerRunning()
		mlBackendOK := svcManager.IsMLBackendRunning()
		sendJSON(SystemInfoResponse{
			Type:              "system_info",
			WhisperServer:     whisperOK,
			TranslationEngine: llamaOK,
			MLBackend:         mlBackendOK,
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

	case "check_setup":
		result := CheckSetup(svcManager.config, actionRegistry)
		sendJSON(result)

	case "install_dependency":
		if cmd.ActionID == "" {
			sendError("Invalid command", "install_dependency requires action_id")
			return
		}
		go runInstallDependency(cmd.ActionID, svcManager)

	default:
		sendError("Unknown command", fmt.Sprintf("command '%s' is not recognized", cmd.Command))
	}
}

func runInstallDependency(actionID string, svcManager *ServiceManager) {
	if !pipelineMu.TryLock() {
		sendError("Install failed", "Cannot install while processing. Stop processing first.")
		return
	}
	defer pipelineMu.Unlock()

	action, err := actionRegistry.Resolve(actionID)
	if err != nil {
		sendError("Install failed", err.Error())
		return
	}

	if err := DownloadAndExtractArchive(action, svcManager, func(downloaded, total int64) {
		if total > 0 {
			pct := float64(downloaded) / float64(total) * 100
			sendProgress("downloading_dependency", pct, fmt.Sprintf("Downloading %s / %s", formatBytes(downloaded), formatBytes(total)))
		} else {
			sendProgress("downloading_dependency", 0, fmt.Sprintf("Downloading %s...", formatBytes(downloaded)))
		}
	}); err != nil {
		sendError("Install failed", err.Error())
		return
	}

	sendProgress("validating", 100, "Installation complete")

	// Re-check setup and send fresh status
	result := CheckSetup(svcManager.config, actionRegistry)
	sendJSON(result)
}

func listAvailableLanguages() ([]LanguagePair, error) {
	capabilities := buildCapabilitiesResponse(NewServiceManager(DefaultServiceConfig()))
	targets := capabilities.Backends.Translation
	if len(targets) == 0 {
		return StaticLanguagePairs(), nil
	}

	var targetLanguages []LanguageOption
	for _, backend := range targets {
		if backend.ID == capabilities.Defaults.TranslationBackend && len(backend.TargetLanguages) > 0 {
			targetLanguages = backend.TargetLanguages
			break
		}
	}
	if len(targetLanguages) == 0 {
		return StaticLanguagePairs(), nil
	}

	sourceLanguages := append([]LanguageOption{{Code: "auto", Name: "Auto-detect"}}, commonLanguageOptions()...)
	pairs := make([]LanguagePair, 0, len(sourceLanguages)*len(targetLanguages))
	for _, source := range sourceLanguages {
		if source.Code == "auto" {
			continue
		}
		for _, target := range targetLanguages {
			if source.Code == target.Code {
				continue
			}
			pairs = append(pairs, LanguagePair{Source: source.Code, Target: target.Code})
		}
	}
	return pairs, nil
}

func buildCapabilitiesResponse(svcManager *ServiceManager) CapabilitiesResponse {
	capabilities := defaultCapabilities()

	if svcManager == nil {
		return capabilities
	}

	for i := range capabilities.Backends.ASR {
		if capabilities.Backends.ASR[i].ID == "whisper_cpp" {
			whisperBinary, whisperModel := resolveWhisperAssets(svcManager.config.SearchRoots, "base")
			capabilities.Backends.ASR[i].Installed = validateWhisperStartup(svcManager.config.SearchRoots, whisperBinary, whisperModel) == nil
		}
	}
	for i := range capabilities.Backends.Translation {
		if capabilities.Backends.Translation[i].ID == gemmaTranslationBackend {
			capabilities.Backends.Translation[i].Installed = resolveGemmaModelPath(svcManager.config.SearchRoots) != ""
		}
	}

	if !fileExists(resolveMLBackendLauncher(svcManager.config.SearchRoots)) && resolveMLBackendScriptPath(svcManager.config.SearchRoots) == "" {
		return capabilities
	}
	if err := svcManager.StartMLBackend(); err != nil {
		return capabilities
	}

	client := NewMLBackendClient(svcManager.MLBackendURL())
	remote, err := client.Capabilities()
	if err != nil {
		return capabilities
	}

	mergeCapabilities(&capabilities, remote)
	return capabilities
}

func mergeCapabilities(base *CapabilitiesResponse, remote *CapabilitiesResponse) {
	if base == nil || remote == nil {
		return
	}
	base.Defaults = remote.Defaults

	for _, remoteASR := range remote.Backends.ASR {
		replaced := false
		for i := range base.Backends.ASR {
			if base.Backends.ASR[i].ID == remoteASR.ID {
				base.Backends.ASR[i] = remoteASR
				replaced = true
				break
			}
		}
		if !replaced {
			base.Backends.ASR = append(base.Backends.ASR, remoteASR)
		}
	}

	for _, remoteTranslation := range remote.Backends.Translation {
		replaced := false
		for i := range base.Backends.Translation {
			if base.Backends.Translation[i].ID == remoteTranslation.ID {
				base.Backends.Translation[i] = remoteTranslation
				replaced = true
				break
			}
		}
		if !replaced {
			base.Backends.Translation = append(base.Backends.Translation, remoteTranslation)
		}
	}
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
	return parseVRAMInfo(out)
}

func parseVRAMInfo(out string) *VRAMInfo {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) == 0 {
		return nil
	}

	info := &VRAMInfo{}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.Split(line, ",")
		if len(parts) != 3 {
			return nil
		}

		total, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
		used, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
		free, err3 := strconv.Atoi(strings.TrimSpace(parts[2]))
		if err1 != nil || err2 != nil || err3 != nil {
			return nil
		}

		info.TotalMiB += total
		info.UsedMiB += used
		info.FreeMiB += free
	}

	if info.TotalMiB == 0 {
		return nil
	}

	return info
}
