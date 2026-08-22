package main

import (
	"math"
	"strings"
	"testing"
)

func almostEqual(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func seg(start, end float64, text string) Segment {
	return Segment{Start: start, End: end, Text: text}
}

func TestNewTimingOptionsLanguageDefaults(t *testing.T) {
	en := NewTimingOptions("en", 24)
	if en.MinDur != 5.0/6.0 || en.MaxDur != 7.0 {
		t.Fatalf("latin duration defaults wrong: %+v", en)
	}
	if en.Gap != 2.0/24.0 || en.SnapWindow != 12.0/24.0 {
		t.Fatalf("latin gap/snap defaults wrong: %+v", en)
	}
	if en.CPSTarget != 20 || en.MaxCols != 42 || en.CJK {
		t.Fatalf("latin cps/cols/cjk wrong: %+v", en)
	}

	ja := NewTimingOptions("ja", 24)
	if !ja.CJK || ja.CPSTarget != 9 || ja.MaxCols != cjkLineColumns {
		t.Fatalf("cjk options wrong: %+v", ja)
	}

	noFps := NewTimingOptions("", 0)
	if noFps.FrameRate != 24 {
		t.Fatalf("frame rate fallback = %v, want 24", noFps.FrameRate)
	}
}

func TestNormalizeCuesResolvesOverlapsAndSorts(t *testing.T) {
	opts := NewTimingOptions("en", 24)
	cues := NormalizeCues([]Segment{
		seg(3.0, 4.0, "b"),
		seg(1.0, 1.6, "a"),
	}, opts, false)

	if len(cues) != 2 {
		t.Fatalf("len(cues) = %d, want 2", len(cues))
	}
	if cues[0].Text != "a" || cues[1].Text != "b" {
		t.Fatalf("cue order wrong: %#v", cues)
	}
	if cues[0].End > cues[1].Start+timeEpsilon {
		t.Fatalf("overlap not resolved: end=%v nextStart=%v", cues[0].End, cues[1].Start)
	}
	// Short cue should have been extended into the gap up to MinDur.
	if cues[0].End-cues[0].Start < opts.MinDur-timeEpsilon && cues[1].Start-opts.Gap > cues[0].End+timeEpsilon {
		t.Fatalf("short cue not extended despite room: %#v", cues[0])
	}
}

func TestNormalizeCuesSplitsLongCueAtSentenceBoundary(t *testing.T) {
	opts := NewTimingOptions("en", 24)
	long := seg(10.0, 20.0,
		"This is the first sentence which stands alone nicely. And here comes a second sentence that is also quite long indeed.")
	cues := NormalizeCues([]Segment{long}, opts, false)

	if len(cues) < 2 {
		t.Fatalf("expected split into >=2 cues, got %d", len(cues))
	}
	for _, cue := range cues {
		if cue.End-cue.Start > opts.MaxDur+timeEpsilon {
			t.Fatalf("cue exceeds max duration: %#v", cue)
		}
		if !strings.Contains(cue.Text, ".") == false && len(cue.Text) == 0 {
			t.Fatalf("empty cue text: %#v", cue)
		}
	}
	// Times must stay contiguous and ordered.
	for i := 0; i+1 < len(cues); i++ {
		if cues[i].End > cues[i+1].Start+timeEpsilon {
			t.Fatalf("split produced overlap: %#v vs %#v", cues[i], cues[i+1])
		}
	}
}

func TestSplitUsesWordTimestampsForBoundary(t *testing.T) {
	opts := NewTimingOptions("en", 24)
	s := Segment{
		Start: 0, End: 10,
		Text: "aaaa bbbb",
		Words: []Word{
			{Text: "aaaa", Start: 0.0, End: 2.0},
			{Text: "bbbb", Start: 8.0, End: 10.0},
		},
	}
	left, right, ok := splitCueOnce(s, opts)
	if !ok {
		t.Fatal("splitCueOnce() failed")
	}
	if !almostEqual(left.End, 5.0) || !almostEqual(right.Start, 5.0) {
		t.Fatalf("boundary = (%v,%v), want midpoint (5.0,5.0)", left.End, right.Start)
	}
	if len(left.Words) != 1 || left.Words[0].Text != "aaaa" {
		t.Fatalf("left words partitioned wrong: %#v", left.Words)
	}
	if len(right.Words) != 1 || right.Words[0].Text != "bbbb" {
		t.Fatalf("right words partitioned wrong: %#v", right.Words)
	}
}

func TestMergeFragmentsJoinsStutterOutput(t *testing.T) {
	opts := NewTimingOptions("en", 24)
	cues := NormalizeCues([]Segment{
		seg(0.0, 0.4, "Yeah"),
		seg(0.45, 0.9, "yeah!"),
		seg(2.0, 3.5, "That was great."),
	}, opts, false)

	if len(cues) != 2 {
		t.Fatalf("len(cues) = %d, want 2 after merge: %#v", len(cues), cues)
	}
	if cues[0].Text != "Yeah yeah!" {
		t.Fatalf("merged text = %q", cues[0].Text)
	}
	if cues[0].End != cues[1].Start-opts.Gap && cues[0].End > cues[1].Start {
		t.Fatalf("gap enforcement failed: %#v", cues[0])
	}
}

func TestEnforceGapsShrinksEarlierEnd(t *testing.T) {
	opts := NewTimingOptions("en", 24)
	cues := []Segment{
		seg(0.0, 5.0, "first"),    // long enough to shrink
		seg(5.001, 7.0, "second"), // only 1ms gap
	}
	out := enforceGaps(cues, opts)
	gap := out[1].Start - out[0].End
	if gap < opts.Gap-timeEpsilon {
		t.Fatalf("gap = %v, want >= %v", gap, opts.Gap)
	}
	if out[0].End-out[0].Start < defaultMinDurationSec*0.5 {
		t.Fatalf("shrink floor violated: %#v", out[0])
	}
}

func TestSnapToShotsSnapsEndBeforeCut(t *testing.T) {
	opts := NewTimingOptions("en", 24)
	opts.ShotTimes = []float64{6.05}
	cues := snapToShots([]Segment{
		seg(0.0, 5.9, "hello there"),
	}, opts)

	if !almostEqual(cues[0].End, 6.05-opts.Gap) {
		t.Fatalf("end = %v, want %v", cues[0].End, 6.05-opts.Gap)
	}
	if cues[0].Start != 0.0 {
		t.Fatalf("start moved unexpectedly: %v", cues[0].Start)
	}
}

func TestSnapToShotsSnapsStartOnCut(t *testing.T) {
	opts := NewTimingOptions("en", 24)
	opts.ShotTimes = []float64{10.0}
	cues := snapToShots([]Segment{
		seg(10.15, 14.0, "after the cut dialogue line"),
	}, opts)

	if !almostEqual(cues[0].Start, 10.0) {
		t.Fatalf("start = %v, want 10.0", cues[0].Start)
	}
}

func TestRoundFramesQuantizesAndKeepsOrder(t *testing.T) {
	opts := NewTimingOptions("en", 24)
	cues := roundFrames([]Segment{
		seg(0.50004, 1.99996, "a"),
		seg(2.00001, 3.49999, "b"),
	}, opts)

	if !almostEqual(cues[0].Start, 0.5) {
		t.Fatalf("start = %v, want 0.5", cues[0].Start)
	}
	if cues[0].End > cues[1].Start {
		t.Fatalf("rounding broke ordering: %#v", cues)
	}
}

func TestComputeQCReportsViolations(t *testing.T) {
	opts := NewTimingOptions("en", 24)
	cues := []Segment{
		{Start: 0.0, End: 0.1, Text: "tiny"},                      // under min
		{Start: 0.2, End: 8.5, Text: strings.Repeat("word ", 40)}, // over duration + cps
		seg(8.55, 10.0, "next"),                                   // tiny gap
		{Start: 10.0, End: 11.0, Text: strings.Repeat("x", 50), Lines: []string{strings.Repeat("x", 50)}}, // line too long
	}

	report := ComputeQC(cues, opts)
	if report.Summary["duration_under_min"] != 1 {
		t.Fatalf("duration_under_min = %v", report.Summary)
	}
	if report.Summary["duration_over_max"] != 1 {
		t.Fatalf("duration_over_max = %v", report.Summary)
	}
	if report.Summary["line_too_long"] != 1 {
		t.Fatalf("line_too_long = %v", report.Summary)
	}
	if report.MaxCPS <= opts.CPSTarget {
		t.Fatalf("max CPS = %v, expected above target", report.MaxCPS)
	}
	if report.CueCount != 4 {
		t.Fatalf("cue count = %d, want 4", report.CueCount)
	}
}

func TestChooseSplitPositionHonorsKinsoku(t *testing.T) {
	runes := []rune("前半部分のテキストです。「後半の引用テキストが続きます」以上です。")

	pos := chooseSplitPosition(runes, true)
	if pos == -1 {
		t.Fatal("chooseSplitPosition() found no break")
	}
	// Break must never create a line starting with closing punctuation
	// or ending with an opening bracket.
	startRune := runes[pos]
	endRune := runes[pos-1]
	if kinsokuCannotStart[startRune] {
		t.Fatalf("illegal break before %q at %d", startRune, pos)
	}
	if kinsokuCannotEnd[endRune] {
		t.Fatalf("illegal break after %q at %d", endRune, pos)
	}
}

func TestCharPosToTimeFallsBackProportionally(t *testing.T) {
	s := seg(10.0, 20.0, "abcdefghij")
	if got := charPosToTime(s, 5); !almostEqual(got, 15.0) {
		t.Fatalf("charPosToTime() = %v, want 15.0", got)
	}
}

// --- test helpers ---
