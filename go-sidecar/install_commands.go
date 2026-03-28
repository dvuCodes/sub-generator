package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func runInstallAction(action Action, svcManager *ServiceManager, progress func(downloaded, total int64)) error {
	switch action.Kind {
	case "archive":
		return DownloadAndExtractArchive(action, svcManager, progress)
	case "command":
		return runCommandInstallAction(action, svcManager)
	case "manual":
		return fmt.Errorf("action %q requires manual setup", action.ID)
	default:
		return fmt.Errorf("unsupported install action kind %q", action.Kind)
	}
}

func runCommandInstallAction(action Action, svcManager *ServiceManager) error {
	switch action.Command {
	case "install_ml_backend_requirements":
		return installMLBackendRequirements(svcManager)
	default:
		return fmt.Errorf("unsupported install command %q", action.Command)
	}
}

func installMLBackendRequirements(svcManager *ServiceManager) error {
	svcManager.StopMLBackend()

	pythonPath := resolveMLBackendPython(svcManager.config.SearchRoots)
	if err := validateCommandAvailability(pythonPath, "python"); err != nil {
		return fmt.Errorf("python runtime is required for ml-backend: %w", err)
	}

	requirementsPath := resolveMLBackendRequirementsPath(svcManager.config.SearchRoots)
	if requirementsPath == "" {
		return fmt.Errorf("ml-backend requirements.txt missing under %q", preferredMLBackendInstallDir(svcManager.config.SearchRoots))
	}

	sendStage("installing_dependency", "Installing Python dependencies...")
	cmd := exec.Command(
		pythonPath,
		"-m",
		"pip",
		"install",
		"--disable-pip-version-check",
		"-r",
		requirementsPath,
	)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	cmd.Dir = filepath.Dir(requirementsPath)

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to install Python dependencies from %q: %w", requirementsPath, err)
	}

	sendStage("validating", "Validating Python dependencies...")
	capabilities, err := mlBackendCapabilitiesForSetup(svcManager.config)
	if err != nil {
		return fmt.Errorf("failed to validate ml-backend after dependency install: %w", err)
	}
	if !mlBackendASRInstalled(capabilities) {
		return fmt.Errorf("faster-whisper remains unavailable after dependency install")
	}

	return nil
}
