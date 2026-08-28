package main

import (
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func TestReadWAVDurationComputesFromHeader(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.wav")

	const sampleRate = 16000
	const channels = 1
	const bitsPerSample = 16
	byteRate := uint32(sampleRate * channels * bitsPerSample / 8)
	dataBytes := uint32(byteRate * 5) // 5 seconds

	var buf []byte
	buf = append(buf, "RIFF"...)
	buf = binary.LittleEndian.AppendUint32(buf, 0) // placeholder size
	buf = append(buf, "WAVE"...)

	buf = append(buf, "fmt "...)

	fmtChunk := make([]byte, 16)
	binary.LittleEndian.PutUint16(fmtChunk[0:], 1) // PCM
	binary.LittleEndian.PutUint16(fmtChunk[2:], channels)
	binary.LittleEndian.PutUint32(fmtChunk[4:], sampleRate)
	binary.LittleEndian.PutUint32(fmtChunk[8:], byteRate)
	binary.LittleEndian.PutUint16(fmtChunk[12:], channels*bitsPerSample/8)
	binary.LittleEndian.PutUint16(fmtChunk[14:], bitsPerSample)
	buf = binary.LittleEndian.AppendUint32(buf, uint32(len(fmtChunk)))
	buf = append(buf, fmtChunk...)

	buf = append(buf, "data"...)
	buf = binary.LittleEndian.AppendUint32(buf, dataBytes)
	buf = append(buf, make([]byte, dataBytes)...)

	if err := os.WriteFile(path, buf, 0o644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	duration := readWAVDuration(path)
	if duration < 4.99 || duration > 5.01 {
		t.Fatalf("readWAVDuration() = %v, want ~5.0", duration)
	}
}

func TestParseFFmpegDuration(t *testing.T) {
	stderr := []byte("[info] Input #0, matroska, from 'x.mkv':\n  Duration: 01:02:03.50, start: 0.000000\n")
	got := parseFFmpegDuration(stderr)
	want := float64(1*3600+2*60+3) + 0.5
	if got != want {
		t.Fatalf("parseFFmpegDuration() = %v, want %v", got, want)
	}
	if parseFFmpegDuration([]byte("nothing here")) != 0 {
		t.Fatal("expected 0 for unparsable input")
	}
}

func TestExtractAudioFailsGracefullyWithoutFFmpeg(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	_, _, err := ExtractAudio(context.Background(), "", "whatever.mp4")
	if err == nil {
		t.Fatal("expected error when ffmpeg is missing")
	}
}

func TestDetectShotChangesParsesPtsTime(t *testing.T) {
	// The regex behavior is the contract; verify against sample metadata output.
	sample := "frame:12 pts:288 pts_time:12\nlavfi.scene_score.N=0.42\nframe:48 pts:1152 pts_time:48.04\nlavfi.scene_score.N=0.51\n"
	cuts := parseShotTimes([]byte(sample))
	if len(cuts) != 2 || cuts[0] != 12.0 || cuts[1] != 48.04 {
		t.Fatalf("parseShotTimes() = %v", cuts)
	}
}
