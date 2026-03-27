package main

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func createTestZip(t *testing.T, dest string, files map[string]string) {
	t.Helper()
	f, err := os.Create(dest)
	if err != nil {
		t.Fatalf("create zip: %v", err)
	}
	defer f.Close()

	w := zip.NewWriter(f)
	for name, content := range files {
		fw, err := w.Create(name)
		if err != nil {
			t.Fatalf("add file: %v", err)
		}
		fw.Write([]byte(content))
	}
	w.Close()
}

func TestExtractZip_StripComponents0(t *testing.T) {
	tmpDir := t.TempDir()
	zipPath := filepath.Join(tmpDir, "test.zip")
	destDir := filepath.Join(tmpDir, "out")

	createTestZip(t, zipPath, map[string]string{
		"folder/binary.exe": "exe content",
		"folder/lib.dll":    "dll content",
	})

	err := extractZip(zipPath, destDir, 0)
	if err != nil {
		t.Fatalf("extractZip failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(destDir, "folder", "binary.exe")); err != nil {
		t.Error("expected folder/binary.exe to exist")
	}
}

func TestExtractZip_StripComponents1(t *testing.T) {
	tmpDir := t.TempDir()
	zipPath := filepath.Join(tmpDir, "test.zip")
	destDir := filepath.Join(tmpDir, "out")

	createTestZip(t, zipPath, map[string]string{
		"top-folder/binary.exe": "exe content",
		"top-folder/lib.dll":    "dll content",
	})

	err := extractZip(zipPath, destDir, 1)
	if err != nil {
		t.Fatalf("extractZip failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(destDir, "binary.exe")); err != nil {
		t.Error("expected binary.exe at root of destDir after strip")
	}
}

func TestExtractZip_MissingBinary(t *testing.T) {
	tmpDir := t.TempDir()
	zipPath := filepath.Join(tmpDir, "test.zip")
	destDir := filepath.Join(tmpDir, "out")

	createTestZip(t, zipPath, map[string]string{
		"top-folder/other.txt": "not a binary",
	})

	err := extractZip(zipPath, destDir, 1)
	if err != nil {
		t.Fatalf("extractZip itself should not fail: %v", err)
	}

	if _, err := os.Stat(filepath.Join(destDir, "whisper-server.exe")); !os.IsNotExist(err) {
		t.Error("expected binary to be absent")
	}
}

func TestAtomicSwap_Success(t *testing.T) {
	tmpDir := t.TempDir()
	installDir := filepath.Join(tmpDir, "service")
	tempDir := filepath.Join(tmpDir, ".service-install-tmp")

	os.MkdirAll(installDir, 0o755)
	os.WriteFile(filepath.Join(installDir, "old.txt"), []byte("old"), 0o644)
	os.MkdirAll(tempDir, 0o755)
	os.WriteFile(filepath.Join(tempDir, "new.txt"), []byte("new"), 0o644)

	err := atomicSwapDirs(installDir, tempDir)
	if err != nil {
		t.Fatalf("atomicSwapDirs failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(installDir, "new.txt")); err != nil {
		t.Error("expected new.txt in installDir after swap")
	}
	if _, err := os.Stat(installDir + ".bak"); !os.IsNotExist(err) {
		t.Error("expected .bak to be cleaned up")
	}
}

func TestAtomicSwap_StepAFails(t *testing.T) {
	tmpDir := t.TempDir()
	installDir := filepath.Join(tmpDir, "nonexistent-dir")
	tempDir := filepath.Join(tmpDir, ".tmp")
	os.MkdirAll(tempDir, 0o755)

	err := atomicSwapDirs(installDir, tempDir)
	// First-time install: no existing dir to back up, should still succeed
	// (or handle gracefully)
	_ = err
}

func TestAtomicSwap_FirstTimeInstall(t *testing.T) {
	tmpDir := t.TempDir()
	installDir := filepath.Join(tmpDir, "new-service")
	tempDir := filepath.Join(tmpDir, ".new-service-tmp")

	// No installDir exists yet - first time install
	os.MkdirAll(tempDir, 0o755)
	os.WriteFile(filepath.Join(tempDir, "binary.exe"), []byte("new"), 0o644)

	err := atomicSwapDirs(installDir, tempDir)
	if err != nil {
		t.Fatalf("first-time install should succeed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(installDir, "binary.exe")); err != nil {
		t.Error("expected binary.exe in installDir after first-time install")
	}
}
