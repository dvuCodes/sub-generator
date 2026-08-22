package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const neutralAudioTempDir = "sub-generator-audio"

var stageNeutralAudio = stageNeutralAudioWithFFmpeg

func stageNeutralAudioWithFFmpeg(inputPath string) (string, func(), error) {
	if _, err := os.Stat(inputPath); err != nil {
		return "", nil, fmt.Errorf("input file not accessible: %w", err)
	}

	tempDir := filepath.Join(os.TempDir(), neutralAudioTempDir)
	if err := os.MkdirAll(tempDir, 0o755); err != nil {
		return "", nil, fmt.Errorf("failed to create temp directory: %w", err)
	}

	baseName := strings.TrimSuffix(filepath.Base(inputPath), filepath.Ext(inputPath))
	tempFile, err := os.CreateTemp(tempDir, baseName+"-neutral-*.wav")
	if err != nil {
		return "", nil, fmt.Errorf("failed to allocate staged audio path: %w", err)
	}
	outputPath := tempFile.Name()
	if err := tempFile.Close(); err != nil {
		return "", nil, fmt.Errorf("failed to close staged audio handle: %w", err)
	}

	args := []string{
		"-i", inputPath,
		"-vn",
		"-ac", "1",
		"-ar", "16000",
		"-f", "wav",
		"-y",
		outputPath,
	}

	cmd := exec.Command("ffmpeg", args...)
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		_ = os.Remove(outputPath)
		return "", nil, fmt.Errorf("ffmpeg neutral staging failed: %w", err)
	}

	info, err := os.Stat(outputPath)
	if err != nil {
		_ = os.Remove(outputPath)
		return "", nil, fmt.Errorf("cannot access staged audio output: %w", err)
	}
	if info.Size() == 0 {
		_ = os.Remove(outputPath)
		return "", nil, fmt.Errorf("ffmpeg produced no staged audio for %q", inputPath)
	}

	return outputPath, func() {
		if err := os.Remove(outputPath); err != nil && !os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "warning: failed to clean up staged audio %q: %v\n", outputPath, err)
		}
	}, nil
}

// Audio preprocessing pipeline.
//
// Upstream whisper.cpp accepts only WAV/FLAC/MP3/OGG natively; feeding it
// raw MP4/MKV either fails or depends on server-side ffmpeg. We extract a
// canonical 16 kHz mono PCM WAV ourselves so every container works and
// uploads shrink ~100x. The same ffmpeg pass yields scene-cut times for
// shot-change snapping.

const (
	audioSampleRate = 16000
	audioChannels   = 1
	sceneThreshold  = "0.3"
)

var (
	durationLineRe = regexp.MustCompile(`Duration:\s*(\d+):(\d{2}):(\d{2})\.(\d+)`)
	ptsTimeRe      = regexp.MustCompile(`pts_time:(\d+(?:\.\d+)?)`)
)

// LookupFFmpeg returns an executable path for ffmpeg if available.
func LookupFFmpeg() (string, error) {
	path, err := exec.LookPath("ffmpeg")
	if err != nil {
		return "", fmt.Errorf("ffmpeg not found in PATH - install it (https://ffmpeg.org) and reopen SubGen")
	}
	return path, nil
}

// ExtractAudio converts the input video into 16kHz mono s16le WAV inside a
// fresh temp directory. Returns the wav path, audio duration in seconds and
// the temp directory (caller removes it when done).
func ExtractAudio(ctx context.Context, ffmpegPath, input string) (string, float64, error) {
	tmpDir, err := os.MkdirTemp("", "subgen-audio-*")
	if err != nil {
		return "", 0, fmt.Errorf("failed to create temp dir: %w", err)
	}

	wavPath := filepath.Join(tmpDir, "audio.wav")

	cmd := exec.CommandContext(
		ctx,
		ffmpegPath,
		"-hide_banner", "-nostdin",
		"-y",
		"-i", input,
		"-vn",
		"-map", "a:0?",
		"-ac", strconv.Itoa(audioChannels),
		"-ar", strconv.Itoa(audioSampleRate),
		"-c:a", "pcm_s16le",
		wavPath,
	)

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", 0, fmt.Errorf("failed to pipe ffmpeg stderr: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return "", 0, fmt.Errorf("failed to start ffmpeg: %w", err)
	}

	stderrText, _ := io.ReadAll(stderr)
	waitErr := cmd.Wait()
	if waitErr != nil {
		if ctx.Err() != nil {
			return "", 0, ctx.Err()
		}
		tail := stderrText
		if len(tail) > 2000 {
			tail = tail[len(tail)-2000:]
		}
		return "", 0, fmt.Errorf("ffmpeg failed: %w: %s", waitErr, strings.TrimSpace(string(tail)))
	}

	info, err := os.Stat(wavPath)
	if err != nil || info.Size() < 44 {
		return "", 0, fmt.Errorf("ffmpeg produced no usable audio output")
	}

	duration := readWAVDuration(wavPath)
	if duration <= 0 {
		duration = parseFFmpegDuration(stderrText)
	}

	return wavPath, duration, nil
}

// DetectShotChanges returns scene-cut timestamps using ffmpeg's select/metadata filters.
// Failures are non-fatal: shot snapping is best-effort.
func DetectShotChanges(ctx context.Context, ffmpegPath, input string) ([]float64, error) {
	tmp, err := os.CreateTemp("", "subgen-shots-*.txt")
	if err != nil {
		return nil, fmt.Errorf("failed to create scene metadata file: %w", err)
	}
	metaFile := tmp.Name()
	_ = tmp.Close()
	defer func() { _ = os.Remove(metaFile) }()

	// ffmpeg filtergraph option values split on ':' and treat '\' as an
	// escape character. Quoting the value handles Windows drive colons,
	// backslashes and spaces in one step; embedded quotes are doubled per
	// ffmpeg's quoting rules.
	filterValue := "'" + strings.ReplaceAll(filepath.ToSlash(metaFile), "'", "''") + "'"

	cmd := exec.CommandContext(
		ctx,
		ffmpegPath,
		"-hide_banner", "-nostdin",
		"-i", input,
		"-vf", fmt.Sprintf("select='gt(scene,%s)',metadata=print:file=%s", sceneThreshold, filterValue),
		"-an", "-sn", "-dn",
		"-f", "null", "-",
	)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		return nil, fmt.Errorf("scene detection failed: %w: %s", err, truncateForLog(stderr.String(), 300))
	}

	data, err := os.ReadFile(metaFile)
	if err != nil {
		return nil, fmt.Errorf("failed reading scene metadata: %w", err)
	}

	return parseShotTimes(data), nil
}

func parseShotTimes(data []byte) []float64 {
	var cuts []float64
	for _, match := range ptsTimeRe.FindAllStringSubmatch(string(data), -1) {
		t, err := strconv.ParseFloat(match[1], 64)
		if err == nil && t > 0 {
			cuts = append(cuts, t)
		}
	}
	return cuts
}

// readWAVDuration parses the RIFF header to compute exact duration in seconds.
func readWAVDuration(path string) float64 {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return 0
	}
	fileSize := info.Size()
	if fileSize < 44 {
		return 0
	}

	r := bufio.NewReader(f)

	header := make([]byte, 12)
	if _, err := io.ReadFull(r, header); err != nil {
		return 0
	}
	if string(header[0:4]) != "RIFF" || string(header[8:12]) != "WAVE" {
		return 0
	}

	byteRate := uint32(0)
	for {
		chunkHeader := make([]byte, 8)
		if _, err := io.ReadFull(r, chunkHeader); err != nil {
			return 0
		}
		chunkID := string(chunkHeader[0:4])
		chunkSize := binary.LittleEndian.Uint32(chunkHeader[4:8])

		switch chunkID {
		case "fmt ":
			fmtChunk := make([]byte, chunkSize)
			if _, err := io.ReadFull(r, fmtChunk); err != nil || len(fmtChunk) < 12 {
				return 0
			}
			byteRate = binary.LittleEndian.Uint32(fmtChunk[8:12])
			if byteRate == 0 {
				return 0
			}
		case "data":
			if byteRate == 0 {
				return 0
			}
			// Streaming writers emit 0xFFFFFFFF for unknown sizes; trust
			// nothing beyond the actual file length.
			const unknownSize = uint32(0xFFFFFFFF)
			if chunkSize == unknownSize || int64(chunkSize) > fileSize-44 {
				chunkSize = uint32(fileSize - 44)
			}
			if chunkSize == 0 {
				return 0
			}
			return float64(chunkSize) / float64(byteRate)
		default:
			if _, err := io.CopyN(io.Discard, r, int64(chunkSize)); err != nil {
				return 0
			}
		}
		if chunkSize%2 == 1 {
			if _, err := r.Discard(1); err != nil {
				return 0
			}
		}
	}
}

func parseFFmpegDuration(stderr []byte) float64 {
	match := durationLineRe.FindSubmatch(stderr)
	if match == nil {
		return 0
	}
	hours, _ := strconv.Atoi(string(match[1]))
	mins, _ := strconv.Atoi(string(match[2]))
	secs, _ := strconv.Atoi(string(match[3]))
	frac, _ := strconv.ParseFloat("0."+string(match[4]), 64)
	return float64(hours*3600+mins*60+secs) + frac
}
