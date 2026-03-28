package main

import (
	"testing"
)

func TestStitchSegmentsEmpty(t *testing.T) {
	result := StitchSegments(nil, DefaultStitcherConfig())
	if result != nil {
		t.Errorf("expected nil for empty input, got %d blocks", len(result))
	}
}

func TestStitchSegmentsSingleSegment(t *testing.T) {
	segs := []Segment{{Start: 1.0, End: 2.0, Text: "hello"}}
	blocks := StitchSegments(segs, DefaultStitcherConfig())

	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}
	if len(blocks[0].Segments) != 1 {
		t.Errorf("expected 1 segment in block, got %d", len(blocks[0].Segments))
	}
	if blocks[0].Start != 1.0 || blocks[0].End != 2.0 {
		t.Errorf("expected Start=1.0 End=2.0, got Start=%f End=%f", blocks[0].Start, blocks[0].End)
	}
}

func TestStitchSegmentsGroupsByGap(t *testing.T) {
	segs := []Segment{
		{Start: 0.0, End: 1.0, Text: "a"},
		{Start: 1.5, End: 2.5, Text: "b"}, // gap 0.5s → same block
		{Start: 3.0, End: 4.0, Text: "c"}, // gap 0.5s → same block
		{Start: 7.0, End: 8.0, Text: "d"}, // gap 3.0s → new block
		{Start: 8.5, End: 9.5, Text: "e"}, // gap 0.5s → same block as d
	}
	blocks := StitchSegments(segs, DefaultStitcherConfig())

	if len(blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(blocks))
	}
	if len(blocks[0].Segments) != 3 {
		t.Errorf("block 0: expected 3 segments, got %d", len(blocks[0].Segments))
	}
	if len(blocks[1].Segments) != 2 {
		t.Errorf("block 1: expected 2 segments, got %d", len(blocks[1].Segments))
	}
	if blocks[0].Start != 0.0 || blocks[0].End != 4.0 {
		t.Errorf("block 0: expected Start=0.0 End=4.0, got Start=%f End=%f", blocks[0].Start, blocks[0].End)
	}
	if blocks[1].Start != 7.0 || blocks[1].End != 9.5 {
		t.Errorf("block 1: expected Start=7.0 End=9.5, got Start=%f End=%f", blocks[1].Start, blocks[1].End)
	}
}

func TestStitchSegmentsSplitsOnMaxBlockLength(t *testing.T) {
	cfg := StitcherConfig{GapThreshold: 2.0, MaxBlockLength: 5.0, MaxSegmentsPerBlock: 100}
	segs := []Segment{
		{Start: 0.0, End: 2.0, Text: "a"},
		{Start: 2.5, End: 4.0, Text: "b"},  // span 0→4=4s, fits
		{Start: 4.5, End: 6.0, Text: "c"},  // span 0→6=6s, exceeds 5s → new block
		{Start: 6.5, End: 8.0, Text: "d"},  // span 4.5→8=3.5s, fits
		{Start: 8.5, End: 10.0, Text: "e"}, // span 4.5→10=5.5s, exceeds → new block
	}
	blocks := StitchSegments(segs, cfg)

	if len(blocks) != 3 {
		t.Fatalf("expected 3 blocks, got %d", len(blocks))
	}
	if len(blocks[0].Segments) != 2 {
		t.Errorf("block 0: expected 2 segments, got %d", len(blocks[0].Segments))
	}
	if len(blocks[1].Segments) != 2 {
		t.Errorf("block 1: expected 2 segments, got %d", len(blocks[1].Segments))
	}
	if len(blocks[2].Segments) != 1 {
		t.Errorf("block 2: expected 1 segment, got %d", len(blocks[2].Segments))
	}
}

func TestStitchSegmentsSplitsOnMaxSegmentCount(t *testing.T) {
	cfg := StitcherConfig{GapThreshold: 2.0, MaxBlockLength: 100.0, MaxSegmentsPerBlock: 3}
	segs := []Segment{
		{Start: 0.0, End: 1.0, Text: "a"},
		{Start: 1.5, End: 2.0, Text: "b"},
		{Start: 2.5, End: 3.0, Text: "c"}, // 3 segments → at cap
		{Start: 3.5, End: 4.0, Text: "d"}, // would make 4 → new block
		{Start: 4.5, End: 5.0, Text: "e"},
	}
	blocks := StitchSegments(segs, cfg)

	if len(blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(blocks))
	}
	if len(blocks[0].Segments) != 3 {
		t.Errorf("block 0: expected 3 segments, got %d", len(blocks[0].Segments))
	}
	if len(blocks[1].Segments) != 2 {
		t.Errorf("block 1: expected 2 segments, got %d", len(blocks[1].Segments))
	}
}

func TestStitchSegmentsPreservesOriginalTimestamps(t *testing.T) {
	segs := []Segment{
		{Start: 1.234, End: 2.567, Text: "first"},
		{Start: 3.0, End: 4.891, Text: "second"},
	}
	blocks := StitchSegments(segs, DefaultStitcherConfig())

	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}
	for i, seg := range blocks[0].Segments {
		if seg.Start != segs[i].Start || seg.End != segs[i].End || seg.Text != segs[i].Text {
			t.Errorf("segment %d was modified: got %+v, want %+v", i, seg, segs[i])
		}
	}
}

func TestStitchSegmentsPreservesBlankSegments(t *testing.T) {
	segs := []Segment{
		{Start: 0.0, End: 1.0, Text: "hello"},
		{Start: 1.5, End: 2.0, Text: ""},       // blank
		{Start: 2.5, End: 3.0, Text: "..."},    // punctuation
		{Start: 3.5, End: 4.0, Text: "  \t  "}, // whitespace
		{Start: 4.5, End: 5.0, Text: "goodbye"},
	}
	blocks := StitchSegments(segs, DefaultStitcherConfig())

	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}
	if len(blocks[0].Segments) != 5 {
		t.Errorf("expected all 5 segments preserved (including blanks), got %d", len(blocks[0].Segments))
	}
	if blocks[0].Segments[1].Text != "" {
		t.Errorf("blank segment text was modified")
	}
	if blocks[0].Segments[3].Text != "  \t  " {
		t.Errorf("whitespace segment text was modified")
	}
}

func TestStitchSegmentsCustomConfig(t *testing.T) {
	cfg := StitcherConfig{GapThreshold: 0.5, MaxBlockLength: 3.0, MaxSegmentsPerBlock: 5}
	segs := []Segment{
		{Start: 0.0, End: 1.0, Text: "a"},
		{Start: 1.3, End: 2.0, Text: "b"}, // gap 0.3s → ok
		{Start: 2.8, End: 3.5, Text: "c"}, // gap 0.8s > 0.5 → new block
	}
	blocks := StitchSegments(segs, cfg)

	if len(blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(blocks))
	}
	if len(blocks[0].Segments) != 2 {
		t.Errorf("block 0: expected 2 segments, got %d", len(blocks[0].Segments))
	}
	if len(blocks[1].Segments) != 1 {
		t.Errorf("block 1: expected 1 segment, got %d", len(blocks[1].Segments))
	}
}

func TestStitchSegmentsSplitsOnSpeakerChange(t *testing.T) {
	segs := []Segment{
		{Start: 0.0, End: 1.0, Text: "a", SpeakerID: "spk_1"},
		{Start: 1.3, End: 2.0, Text: "b", SpeakerID: "spk_1"},
		{Start: 2.2, End: 3.0, Text: "c", SpeakerID: "spk_2"},
	}

	blocks := StitchSegments(segs, DefaultStitcherConfig())

	if len(blocks) != 2 {
		t.Fatalf("expected 2 blocks on speaker change, got %d", len(blocks))
	}
	if len(blocks[0].Segments) != 2 || len(blocks[1].Segments) != 1 {
		t.Fatalf("unexpected block sizes: %d and %d", len(blocks[0].Segments), len(blocks[1].Segments))
	}
	if blocks[1].Segments[0].SpeakerID != "spk_2" {
		t.Fatalf("expected second block to start with spk_2, got %#v", blocks[1].Segments[0])
	}
}

func TestStitchSegmentsRealisticAnimeDialogue(t *testing.T) {
	// Models fast-paced anime dialogue: short utterances, variable pauses,
	// one long silence (scene change), then rapid back-and-forth.
	segs := []Segment{
		// Scene 1: conversation
		{Start: 0.0, End: 1.2, Text: "おはようございます"},
		{Start: 1.5, End: 2.8, Text: "田中くん、おはよう"},
		{Start: 3.2, End: 3.8, Text: "えっ"},
		{Start: 4.0, End: 5.5, Text: "今日の会議、知ってる？"},
		{Start: 5.8, End: 7.0, Text: "あ、うん"},
		{Start: 7.3, End: 9.0, Text: "部長が怒ってたよ"},
		// Long pause → scene change
		{Start: 15.0, End: 17.0, Text: "行った"},
		{Start: 17.3, End: 18.5, Text: "誰が？"},
		{Start: 18.8, End: 20.0, Text: "田中くんだよ"},
		// Another pause
		{Start: 25.0, End: 26.0, Text: "そうか"},
		{Start: 26.3, End: 27.5, Text: "まあね"},
		{Start: 27.8, End: 29.0, Text: "でも"},
		{Start: 29.3, End: 30.5, Text: "仕方ないよ"},
	}

	blocks := StitchSegments(segs, DefaultStitcherConfig())

	// Expect 3 blocks: scene 1 (6 segments), scene 2 (3 segments), scene 3 (4 segments)
	if len(blocks) != 3 {
		t.Fatalf("expected 3 blocks for anime dialogue, got %d", len(blocks))
	}

	if len(blocks[0].Segments) != 6 {
		t.Errorf("block 0 (scene 1): expected 6 segments, got %d", len(blocks[0].Segments))
	}
	if blocks[0].Start != 0.0 || blocks[0].End != 9.0 {
		t.Errorf("block 0: unexpected boundaries Start=%f End=%f", blocks[0].Start, blocks[0].End)
	}

	if len(blocks[1].Segments) != 3 {
		t.Errorf("block 1 (scene 2): expected 3 segments, got %d", len(blocks[1].Segments))
	}
	if blocks[1].Start != 15.0 || blocks[1].End != 20.0 {
		t.Errorf("block 1: unexpected boundaries Start=%f End=%f", blocks[1].Start, blocks[1].End)
	}

	if len(blocks[2].Segments) != 4 {
		t.Errorf("block 2 (scene 3): expected 4 segments, got %d", len(blocks[2].Segments))
	}
	if blocks[2].Start != 25.0 || blocks[2].End != 30.5 {
		t.Errorf("block 2: unexpected boundaries Start=%f End=%f", blocks[2].Start, blocks[2].End)
	}

	// Verify total segment count is preserved
	totalSegs := 0
	for _, b := range blocks {
		totalSegs += len(b.Segments)
	}
	if totalSegs != len(segs) {
		t.Errorf("total segments across blocks (%d) != input segments (%d)", totalSegs, len(segs))
	}
}

func TestStitchSegmentsExactCapBoundary(t *testing.T) {
	// Verify that exactly MaxSegmentsPerBlock segments fit in one block
	cfg := StitcherConfig{GapThreshold: 2.0, MaxBlockLength: 100.0, MaxSegmentsPerBlock: 3}
	segs := []Segment{
		{Start: 0.0, End: 1.0, Text: "a"},
		{Start: 1.5, End: 2.0, Text: "b"},
		{Start: 2.5, End: 3.0, Text: "c"},
	}
	blocks := StitchSegments(segs, cfg)

	if len(blocks) != 1 {
		t.Fatalf("expected exactly 1 block for %d segments at cap %d, got %d blocks", len(segs), cfg.MaxSegmentsPerBlock, len(blocks))
	}
	if len(blocks[0].Segments) != 3 {
		t.Errorf("expected 3 segments in block, got %d", len(blocks[0].Segments))
	}
}
