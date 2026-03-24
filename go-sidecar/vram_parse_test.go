package main

import "testing"

func TestParseVRAMInfoSumsMultipleGPUs(t *testing.T) {
	info := parseVRAMInfo("8192, 1024, 7168\n16384, 4096, 12288")
	if info == nil {
		t.Fatal("parseVRAMInfo() = nil, want totals")
	}
	if info.TotalMiB != 24576 || info.UsedMiB != 5120 || info.FreeMiB != 19456 {
		t.Fatalf("parseVRAMInfo() = %#v, want summed totals", info)
	}
}

func TestParseVRAMInfoRejectsMalformedRows(t *testing.T) {
	info := parseVRAMInfo("8192, 1024\nbad-data")
	if info != nil {
		t.Fatalf("parseVRAMInfo() = %#v, want nil for malformed output", info)
	}
}
