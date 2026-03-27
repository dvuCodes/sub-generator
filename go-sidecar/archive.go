package main

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func extractZip(zipPath, destDir string, stripComponents int) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("failed to open zip: %w", err)
	}
	defer r.Close()

	for _, f := range r.File {
		name := f.Name
		if stripComponents > 0 {
			parts := strings.SplitN(name, "/", stripComponents+1)
			if len(parts) <= stripComponents {
				continue
			}
			name = parts[stripComponents]
			if name == "" {
				continue
			}
		}

		targetPath := filepath.Join(destDir, filepath.FromSlash(name))

		// Prevent zip slip
		if !strings.HasPrefix(filepath.Clean(targetPath), filepath.Clean(destDir)+string(os.PathSeparator)) {
			continue
		}

		if f.FileInfo().IsDir() {
			os.MkdirAll(targetPath, 0o755)
			continue
		}

		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			return fmt.Errorf("failed to create directory for %s: %w", name, err)
		}

		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("failed to open zip entry %s: %w", name, err)
		}

		outFile, err := os.Create(targetPath)
		if err != nil {
			rc.Close()
			return fmt.Errorf("failed to create %s: %w", targetPath, err)
		}

		_, err = io.Copy(outFile, rc)
		outFile.Close()
		rc.Close()
		if err != nil {
			return fmt.Errorf("failed to write %s: %w", targetPath, err)
		}
	}

	return nil
}

func atomicSwapDirs(installDir, tempDir string) error {
	bakDir := installDir + ".bak"

	// Clean up any stale backup
	os.RemoveAll(bakDir)

	// Check if installDir exists (first-time install has no existing dir)
	_, existsErr := os.Stat(installDir)
	hasExisting := existsErr == nil

	if hasExisting {
		// Step A: rename current → .bak
		if err := os.Rename(installDir, bakDir); err != nil {
			return fmt.Errorf("install_locked: cannot move current directory (is a file in use?): %w", err)
		}
	}

	// Step B: rename temp → installDir
	if err := os.Rename(tempDir, installDir); err != nil {
		if hasExisting {
			restoreErr := os.Rename(bakDir, installDir)
			if restoreErr != nil {
				return fmt.Errorf("install_locked: failed to install AND failed to restore backup: move=%w restore=%v", err, restoreErr)
			}
		}
		return fmt.Errorf("install_locked: cannot place new directory: %w", err)
	}

	// Success — clean up backup if it exists
	if hasExisting {
		os.RemoveAll(bakDir)
	}
	return nil
}

func DownloadAndExtractArchive(
	action Action,
	svcManager *ServiceManager,
	progress func(downloaded, total int64),
) error {
	// Stop managed service for this action's service
	switch action.ServiceID {
	case "whisper":
		svcManager.StopWhisperServer()
	case "llama":
		svcManager.StopLlamaServer()
	}

	// Download to temp file
	tmpZip := filepath.Join(os.TempDir(), "subgen-install-"+action.ServiceID+".zip")
	defer os.Remove(tmpZip)

	sendStage("downloading_dependency", fmt.Sprintf("Downloading %s...", action.Label))
	if err := DownloadModel(action.URL, tmpZip, progress); err != nil {
		return fmt.Errorf("download failed: %w", err)
	}

	// Extract to temp sibling directory (same volume for rename)
	parentDir := filepath.Dir(action.InstallDir)
	tempExtractDir := filepath.Join(parentDir, "."+filepath.Base(action.InstallDir)+"-install-tmp")
	os.RemoveAll(tempExtractDir)
	defer os.RemoveAll(tempExtractDir)

	sendStage("extracting", "Extracting files...")
	if err := extractZip(tmpZip, tempExtractDir, action.StripComponents); err != nil {
		return fmt.Errorf("extraction failed: %w", err)
	}

	// Validate expected binary exists
	sendStage("validating", "Validating installation...")
	expectedPath := filepath.Join(tempExtractDir, action.ExpectedBinary)
	if _, err := os.Stat(expectedPath); err != nil {
		return fmt.Errorf("installation validation failed: expected binary %q not found in extracted archive", action.ExpectedBinary)
	}

	// Atomic swap
	if err := atomicSwapDirs(action.InstallDir, tempExtractDir); err != nil {
		return err
	}

	return nil
}
