package main

import (
	"math"
	"sort"
	"strings"
	"unicode"
)

// Cue normalization engine.
//
// Converts raw transcription segments into broadcast-quality subtitle cues
// following Netflix Timed Text Style Guide conventions:
//
//   - minimum duration 5/6 s (20 frames @24fps), maximum 7 s
//   - reading-speed driven splitting (CPS limits per language)
//   - 2-frame gaps between consecutive cues, no overlaps
//   - scene-cut snapping within a 12-frame window
//   - optional frame-grid rounding

const (
	defaultMinDurationSec = 5.0 / 6.0
	defaultMaxDurationSec = 7.0
	framesGap             = 2.0
	framesSnapWindow      = 12.0

	maxSplitDepth = 6
	minChunkUnits = 6

	timeEpsilon = 1e-3
)

// TimingOptions configures the normalization passes. All values are seconds
// unless noted otherwise.
type TimingOptions struct {
	MinDur     float64
	MaxDur     float64
	Gap        float64 // minimum gap between consecutive cues
	CPSTarget  float64 // reading speed limit used for splitting decisions
	MaxCols    int     // max line width in half-width columns
	MaxLines   int     // max display lines per cue
	FrameRate  float64 // 0 disables frame rounding
	SnapWindow float64 // max distance to a scene cut eligible for snapping
	CJK        bool    // target language uses wide glyphs

	// ShotTimes holds scene-cut timestamps (seconds, ascending) for snapping.
	ShotTimes []float64
}

// NewTimingOptions builds language-aware defaults from a target language code
// ("", "ja", "en", ...) and an optional video frame rate (0 = assume 24).
func NewTimingOptions(targetLang string, frameRate float64) TimingOptions {
	cjk := targetLang != "" && isCJKLanguage(targetLang)

	cps := 20.0 // Netflix adult reading speed for latin targets
	cols := defaultLatinLineColumns
	if cjk {
		cps = 9.0 // conservative Japanese broadcast ceiling
		cols = cjkLineColumns
	}
	if frameRate <= 0 {
		frameRate = 24
	}

	return TimingOptions{
		MinDur:     defaultMinDurationSec,
		MaxDur:     defaultMaxDurationSec,
		Gap:        framesGap / frameRate,
		CPSTarget:  cps,
		MaxCols:    cols,
		MaxLines:   2,
		FrameRate:  frameRate,
		SnapWindow: framesSnapWindow / frameRate,
		CJK:        cjk,
	}
}

// NormalizeCues runs the full normalization chain on segment copies.
func NormalizeCues(segments []Segment, opts TimingOptions, snapShots bool) []Segment {
	cues := sanitizeCues(segments)
	if len(cues) == 0 {
		return nil
	}

	cues = mergeFragments(cues, opts)
	cues = splitLongCues(cues, opts)
	cues = enforceDurations(cues, opts)
	cues = enforceGaps(cues, opts)
	if snapShots && len(opts.ShotTimes) > 0 {
		cues = snapToShots(cues, opts)
	}
	cues = roundFrames(cues, opts)

	return dropSubFrameCues(cues)
}

// dropSubFrameCues removes cues too short to display on one video frame.
func dropSubFrameCues(cues []Segment) []Segment {
	out := make([]Segment, 0, len(cues))
	for _, cue := range cues {
		if cue.End-cue.Start >= timeEpsilon {
			out = append(out, cue)
		}
	}
	return out
}

func cloneSegment(s Segment) Segment {
	clone := s
	clone.Words = append([]Word(nil), s.Words...)
	return clone
}

// sanitizeCues drops empty cues, repairs invalid times, sorts by start and
// removes overlaps by clamping ends.
func sanitizeCues(segments []Segment) []Segment {
	cues := make([]Segment, 0, len(segments))
	for _, s := range segments {
		s := cloneSegment(s)
		s.Text = strings.TrimSpace(s.Text)
		if s.Text == "" {
			continue
		}
		if s.End < s.Start {
			s.Start, s.End = s.End, s.Start
		}
		if s.End-s.Start < 0.05 {
			s.End = s.Start + 0.25
		}
		cues = append(cues, s)
	}
	sort.SliceStable(cues, func(i, j int) bool { return cues[i].Start < cues[j].Start })

	for i := 0; i+1 < len(cues); i++ {
		if cues[i].End > cues[i+1].Start-timeEpsilon {
			cues[i].End = math.Max(cues[i].Start, cues[i+1].Start-timeEpsilon)
		}
	}
	return cues
}

// mergeFragments joins tiny neighboring cues (common Whisper stutter output)
// whenever the combination stays within duration/CPS limits.
func mergeFragments(cues []Segment, opts TimingOptions) []Segment {
	const mergeGapMax = 0.35

	out := make([]Segment, 0, len(cues))
	for _, cue := range cues {
		if len(out) > 0 {
			prev := &out[len(out)-1]
			gap := cue.Start - prev.End
			combinedDuration := cue.End - prev.Start
			combinedUnits := cueUnits(*prev, opts) + cueUnits(cue, opts)
			combinedCPS := combinedUnits / math.Max(combinedDuration, timeEpsilon)

			if gap >= -timeEpsilon && gap <= mergeGapMax &&
				combinedDuration <= opts.MaxDur &&
				combinedCPS <= opts.CPSTarget &&
				// Merging must not create a cue that cannot fit MaxLines.
				combinedUnits <= maxUnitsPerCue(opts) {
				joiner := " "
				if opts.CJK {
					joiner = ""
				}
				prev.Text += joiner + cue.Text
				prev.SourceText += joiner + cue.SourceText
				prev.Lines = nil
				prev.End = cue.End
				// Inherit the worst-case confidence signals.
				prev.NoSpeechProb = math.Max(prev.NoSpeechProb, cue.NoSpeechProb)
				prev.AvgLogprob = math.Min(prev.AvgLogprob, cue.AvgLogprob)
				prev.Words = append(prev.Words, cue.Words...)
				continue
			}
		}
		out = append(out, cue)
	}
	return out
}

// splitLongCues enforces max duration and CPS targets via recursive
// punctuation-aware splitting.
func splitLongCues(cues []Segment, opts TimingOptions) []Segment {
	out := make([]Segment, 0, len(cues)+8)
	var split func(seg Segment, depth int)
	split = func(seg Segment, depth int) {
		if depth >= maxSplitDepth || !needsSplit(seg, opts) {
			out = append(out, seg)
			return
		}
		left, right, ok := splitCueOnce(seg, opts)
		if !ok {
			out = append(out, seg)
			return
		}
		split(left, depth+1)
		split(right, depth+1)
	}
	for _, cue := range cues {
		split(cue, 0)
	}
	return out
}

func needsSplit(seg Segment, opts TimingOptions) bool {
	duration := seg.End - seg.Start
	if duration > opts.MaxDur+timeEpsilon {
		return true
	}
	units := cueUnits(seg, opts)
	cps := units / math.Max(duration, timeEpsilon)
	if cps > opts.CPSTarget+timeEpsilon {
		return true
	}
	// Line-count feasibility: text whose wrapped width cannot fit within
	// MaxLines display lines must be split even at a legal reading speed.
	return units > maxUnitsPerCue(opts)
}

// maxUnitsPerCue converts the column budget into CueTextUnits, where each
// wide CJK glyph counts once but occupies two columns (13 glyphs/line at 26
// columns).
func maxUnitsPerCue(opts TimingOptions) float64 {
	colsPerLine := opts.MaxCols
	if opts.CJK {
		colsPerLine /= 2
	}
	return float64(colsPerLine * opts.MaxLines)
}

// splitCueOnce divides one cue into two at the best available boundary.
func splitCueOnce(seg Segment, opts TimingOptions) (Segment, Segment, bool) {
	runes := []rune(seg.Text)
	if len(runes) < 8 {
		return seg, Segment{}, false
	}

	pos := chooseSplitPosition(runes, opts.CJK)
	if pos <= 0 || pos >= len(runes) {
		return seg, Segment{}, false
	}

	boundaryTime := charPosToTime(seg, pos)

	leftText := strings.TrimSpace(string(runes[:pos]))
	rightText := strings.TrimSpace(string(runes[pos:]))
	if leftText == "" || rightText == "" {
		return seg, Segment{}, false
	}

	leftWords, rightWords := partitionWords(seg.Words, boundaryTime)

	left := Segment{
		Start:        seg.Start,
		End:          clampTime(boundaryTime, seg.Start+minChunkTime(), seg.End),
		Text:         leftText,
		SourceText:   seg.SourceText,
		Words:        leftWords,
		AvgLogprob:   seg.AvgLogprob,
		NoSpeechProb: seg.NoSpeechProb,
	}
	right := Segment{
		Start:        left.End,
		End:          seg.End,
		Text:         rightText,
		SourceText:   seg.SourceText,
		Words:        rightWords,
		AvgLogprob:   seg.AvgLogprob,
		NoSpeechProb: seg.NoSpeechProb,
	}

	if left.End-left.Start < timeEpsilon || right.End-right.Start < timeEpsilon {
		return seg, Segment{}, false
	}
	return left, right, true
}

const minChunkSeconds = 0.35

func minChunkTime() float64 { return minChunkSeconds }

// chooseSplitPosition finds the best rune index for dividing text.
// Lower priority values are better: sentence enders beat clause separators
// beat whitespace beats arbitrary positions. Kinsoku-invalid breaks are
// skipped entirely for CJK. The tightest middle band is searched first; good
// structural breaks (priority <= 2) win immediately from the first band that
// contains one, while arbitrary mid-word breaks (CJK priority 3) are only
// used as a last resort across all bands.
func chooseSplitPosition(runes []rune, cjk bool) int {
	n := len(runes)
	mid := n / 2

	bands := [][2]int{{n * 30 / 100, n * 70 / 100}, {n / 5, n * 4 / 5}, {n / 10, n * 9 / 10}}

	fallbackPos, fallbackPri, fallbackDist := -1, math.MaxInt, math.MaxInt

	for _, band := range bands {
		best, bestPri, bestDist := -1, math.MaxInt, math.MaxInt
		for pos := maxInt(band[0], 1); pos < minInt(band[1], n-1); pos++ {
			pri, ok := breakPriority(runes, pos, cjk)
			if !ok {
				continue
			}
			dist := absInt(pos - mid)
			if pri < bestPri || (pri == bestPri && dist < bestDist) {
				best, bestPri, bestDist = pos, pri, dist
			}
		}
		if best == -1 {
			continue
		}
		if bestPri <= 2 {
			return best
		}
		if bestPri < fallbackPri || (bestPri == fallbackPri && bestDist < fallbackDist) {
			fallbackPos, fallbackPri, fallbackDist = best, bestPri, bestDist
		}
	}
	return fallbackPos
}

var (
	sentenceEnders = map[rune]bool{'.': true, '!': true, '?': true, '…': true, '。': true, '！': true, '？': true}
	clauseBreakers = map[rune]bool{',': true, ';': true, ':': true, '、': true, '，': true, '；': true, '：': true}
)

// breakPriority reports whether breaking between runes[pos-1] and runes[pos]
// is allowed, and how good that break is (lower is better).
func breakPriority(runes []rune, pos int, cjk bool) (int, bool) {
	endRune := runes[pos-1]
	startRune := runes[pos]

	if kinsokuCannotEnd[endRune] || kinsokuCannotStart[startRune] {
		return 0, false
	}
	switch {
	case sentenceEnders[endRune]:
		return 0, true
	case clauseBreakers[endRune]:
		return 1, true
	case startRune == '"' || startRune == '「' || startRune == '『' || startRune == '(':
		return 1, true
	case unicodeIsSpace(endRune):
		return 2, true
	case cjk:
		return 3, true
	default:
		return 0, false // never split latin words mid-word
	}
}

func unicodeIsSpace(r rune) bool {
	return unicode.IsSpace(r)
}

// charPosToTime maps a character position onto the cue timeline using word
// timestamps when available, falling back to proportional interpolation.
func charPosToTime(seg Segment, pos int) float64 {
	runes := []rune(seg.Text)
	total := len(runes)
	if total == 0 || seg.End <= seg.Start {
		return seg.Start
	}

	words := seg.Words
	if len(words) >= 2 {
		offsets := locateWordOffsets(runes, words)
		for i := 0; i < len(words); i++ {
			if offsets[i] < 0 || pos < offsets[i] {
				continue
			}
			wordEnd := offsets[i] + len([]rune(strings.TrimSpace(words[i].Text)))

			nextStartOffset := len(runes)
			for j := i + 1; j < len(words); j++ {
				if offsets[j] >= 0 {
					nextStartOffset = offsets[j]
					break
				}
			}

			// Position falls within/at this word or in the separator gap
			// between it and the next located word.
			if pos <= nextStartOffset {
				if i+1 < len(words) {
					return clampTime((words[i].End+words[i+1].Start)/2, seg.Start, seg.End)
				}
				local := float64(pos-offsets[i]) / math.Max(float64(wordEnd-offsets[i]), 1)
				t := words[i].Start + local*(words[i].End-words[i].Start)
				return clampTime(t, seg.Start, seg.End)
			}
		}
	}

	fraction := float64(pos) / float64(total)
	return clampTime(seg.Start+fraction*(seg.End-seg.Start), seg.Start, seg.End)
}

// locateWordOffsets finds each word's rune offset within the cue text so
// character positions can be attributed to specific timed tokens.
func locateWordOffsets(runes []rune, words []Word) []int {
	offsets := make([]int, len(words))
	searchFrom := 0
	for i, w := range words {
		offsets[i] = -1
		text := []rune(strings.TrimSpace(w.Text))
		if len(text) == 0 {
			continue
		}
		idx := indexRunes(runes, text, searchFrom)
		if idx >= 0 {
			offsets[i] = idx
			searchFrom = idx + len(text)
		}
	}
	return offsets
}

func indexRunes(haystack, needle []rune, from int) int {
	if len(needle) == 0 {
		return -1
	}
	for i := from; i+len(needle) <= len(haystack); i++ {
		match := true
		for j := range needle {
			if haystack[i+j] != needle[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

func partitionWords(words []Word, boundary float64) ([]Word, []Word) {
	if len(words) == 0 {
		return nil, nil
	}
	idx := sort.Search(len(words), func(i int) bool { return words[i].Start >= boundary })
	if idx == 0 {
		idx = 1
	}
	if idx > len(words) {
		idx = len(words)
	}
	left := append([]Word(nil), words[:idx]...)
	right := append([]Word(nil), words[idx:]...)
	return left, right
}

// enforceDurations extends sub-minimum cues into following gaps and clamps
// over-long leftovers.
func enforceDurations(cues []Segment, opts TimingOptions) []Segment {
	for i := range cues {
		cue := &cues[i]

		limit := opts.MaxDur
		if i+1 < len(cues) {
			limit = math.Min(limit, cues[i+1].Start-opts.Gap)
		}

		if cue.End-cue.Start < opts.MinDur {
			newEnd := math.Min(cue.Start+opts.MinDur, limit)
			if newEnd > cue.End {
				cue.End = newEnd
			}
		}
		if cue.End-cue.Start > opts.MaxDur {
			cue.End = cue.Start + opts.MaxDur
		}
	}
	return cues
}

// enforceGaps guarantees the minimum inter-cue gap, shrinking earlier ends
// where possible, otherwise pushing later starts.
func enforceGaps(cues []Segment, opts TimingOptions) []Segment {
	for i := 0; i+1 < len(cues); i++ {
		cur, next := &cues[i], &cues[i+1]
		gap := next.Start - cur.End
		if gap >= opts.Gap-timeEpsilon {
			continue
		}

		needed := opts.Gap - gap
		shrinkFloor := math.Max(cur.Start, cur.End-0.6*math.Max(cur.End-cur.Start, opts.MinDur))

		newEnd := cur.End - needed
		if newEnd >= shrinkFloor && newEnd > cur.Start {
			cur.End = newEnd
			continue
		}

		newStart := cur.End + opts.Gap
		if newStart < next.End {
			next.Start = newStart
		} else {
			cur.End = math.Max(cur.Start, next.Start-opts.Gap)
		}
	}
	return cues
}

// snapToShots snaps cue edges toward nearby scene cuts following Netflix
// timing zones: ins land on the cut, outs land 2 frames before it. Snapping
// never creates overlaps or violates the minimum gap with adjacent cues.
func snapToShots(cues []Segment, opts TimingOptions) []Segment {
	shots := append([]float64(nil), opts.ShotTimes...)
	sort.Float64s(shots)

	outBeforeCut := framesGap / math.Max(opts.FrameRate, 1)
	minSnapDur := opts.MinDur * 0.5

	for _, cut := range shots {
		for i := range cues {
			cue := &cues[i]

			// Out-time just before a cut.
			if d := cut - cue.End; d > 0 && d <= opts.SnapWindow {
				maxAllowed := cue.Start + opts.MaxDur
				if maxAllowed > cue.End {
					// Skip when the cue already exceeds MaxDur: extending
					// would deepen the violation rather than fix anything.
					newEnd := cut - outBeforeCut
					if i+1 < len(cues) {
						newEnd = math.Min(newEnd, cues[i+1].Start-opts.Gap)
					}
					newEnd = math.Min(newEnd, maxAllowed)
					if newEnd > cue.End && newEnd-cue.Start >= minSnapDur {
						cue.End = newEnd
						continue
					}
				}
			}

			// In-time on a cut.
			if d := cue.Start - cut; d > 0 && d <= opts.SnapWindow {
				newStart := cut
				prevEnd := math.Inf(-1)
				if i > 0 {
					prevEnd = cues[i-1].End
				}
				newDur := cue.End - newStart
				if newStart >= prevEnd+opts.Gap && newDur <= opts.MaxDur && newDur >= opts.MinDur*0.5 {
					cue.Start = newStart
				}
			}
		}
	}
	return cues
}

// roundFrames quantizes times to the video frame grid. Rounding can consume
// part of an enforced gap, so minimum spacing is re-established afterwards.
func roundFrames(cues []Segment, opts TimingOptions) []Segment {
	if opts.FrameRate <= 0 {
		return cues
	}
	fps := opts.FrameRate

	for i := range cues {
		cues[i].Start = math.Round(cues[i].Start*fps) / fps
		cues[i].End = math.Round(cues[i].End*fps) / fps
		if cues[i].End <= cues[i].Start {
			cues[i].End = cues[i].Start + 1/fps
		}
	}

	// Re-establish the minimum gap that rounding may have eroded, then drop
	// anything left degenerate.
	cues = enforceGaps(cues, opts)
	return dropSubFrameCues(cues)
}

// --- QC ---

// ComputeQC audits normalized cues against the timing/formatting rules.
func ComputeQC(cues []Segment, opts TimingOptions) *QCReport {
	report := &QCReport{Summary: map[string]int{}}
	if len(cues) == 0 {
		return report
	}

	// Wrap-fallback language so CJK cues are measured against the correct
	// column budget when Lines have not been finalized.
	lang := cjkLangCode(opts.CJK)

	const maxQCIssues = 200

	report.CueCount = len(cues)
	totalUnits, totalDuration := 0.0, 0.0

	for i := range cues {
		cue := cues[i]
		duration := cue.End - cue.Start
		units := cueUnits(cue, opts)
		cps := units / math.Max(duration, timeEpsilon)
		totalUnits += units
		totalDuration += duration
		if cps > report.MaxCPS {
			report.MaxCPS = cps
		}

		var issues []string
		add := func(name string) {
			issues = append(issues, name)
			report.count(name)
		}

		if duration > opts.MaxDur+timeEpsilon {
			add("duration_over_max")
		}
		if duration < opts.MinDur*0.85 {
			add("duration_under_min")
		}
		if cps > opts.CPSTarget*1.05 {
			add("cps_over_limit")
		}

		lines := cue.Lines
		if len(lines) == 0 {
			lines = WrapCueLines(cue.Text, &lang)
		}
		for _, line := range lines {
			if textWidth(line) > opts.MaxCols {
				add("line_too_long")
				break
			}
		}
		if len(lines) > opts.MaxLines {
			add("lines_over_max")
		}

		if i+1 < len(cues) {
			gap := cues[i+1].Start - cue.End
			switch {
			case gap < -timeEpsilon:
				add("overlap")
			case gap < opts.Gap-timeEpsilon:
				add("gap_too_small")
			}
		}

		// Summary counts stay authoritative for all cues; the issue list is
		// capped to keep the QC payload bounded on pathological files.
		if len(issues) > 0 && len(report.Issues) < maxQCIssues {
			report.Issues = append(report.Issues, QCIssue{
				Index:  i,
				Start:  cue.Start,
				End:    cue.End,
				Issues: issues,
			})
		}
	}

	report.AvgCPS = totalUnits / math.Max(totalDuration, timeEpsilon)
	if report.Summary == nil {
		report.Summary = map[string]int{}
	}
	return report
}

// FinalizeLineWrapping fills in display lines for every cue.
func FinalizeLineWrapping(cues []Segment, targetLang string) {
	langPtr := &targetLang
	for i := range cues {
		if len(cues[i].Lines) == 0 {
			cues[i].Lines = WrapCueLines(cues[i].Text, langPtr)
		}
	}
}

// --- helpers ---

func cueUnits(seg Segment, opts TimingOptions) float64 {
	return CueTextUnits(seg.Text, cjkLangCode(opts.CJK))
}

func cjkLangCode(cjk bool) string {
	if cjk {
		return "ja"
	}
	return "en"
}

func clampTime(v, lo, hi float64) float64 {
	if lo > hi {
		return lo
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
