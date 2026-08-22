package main

import (
	"strings"
	"unicode"

	"github.com/rivo/uniseg"
)

// Subtitle line-wrapping engine.
//
// Targets Netflix Timed Text Style Guide conventions:
//   - Latin targets: 42 characters per line, 2 lines, bottom-heavy pyramid,
//     never stranding function words at line ends.
//   - CJK targets: 13 full-width characters per line (26 columns), with
//     kinsoku shori (禁則処理) rules preventing illegal line starts/ends.

const (
	defaultLatinLineColumns = 42
	cjkLineColumns          = 26 // 13 full-width chars
)

// textWidth returns the display width in half-width columns.
func textWidth(s string) int {
	return uniseg.StringWidth(s)
}

func isWideRune(r rune) bool {
	switch {
	case r >= 0x1100 && r <= 0x115F, // Hangul Jamo
		r >= 0x2E80 && r <= 0x303E,   // CJK radicals, Kangxi, CJK symbols
		r >= 0x3041 && r <= 0x33FF,   // Hiragana, Katakana, CJK punctuation
		r >= 0x3400 && r <= 0x4DBF,   // CJK ext A
		r >= 0x4E00 && r <= 0x9FFF,   // CJK unified
		r >= 0xA000 && r <= 0xA4CF,   // Yi
		r >= 0xAC00 && r <= 0xD7A3,   // Hangul syllables
		r >= 0xF900 && r <= 0xFAFF,   // CJK compatibility ideographs
		r >= 0xFF00 && r <= 0xFF60,   // full-width forms
		r >= 0xFFE0 && r <= 0xFFE6,   // full-width signs
		r >= 0x20000 && r <= 0x3FFFD: // CJK ext B+
		return true
	}
	return false
}

func containsWideRunes(s string) bool {
	for _, r := range s {
		if isWideRune(r) {
			return true
		}
	}
	return false
}

// kinsoku sets: JIS X 4051-style line breaking rules.
var (
	// Runes that must not begin a line.
	kinsokuCannotStart = map[rune]bool{
		'、': true, '。': true, '！': true, '？': true, '…': true, '‥': true,
		'，': true, '．': true, '：': true, '；': true,
		'!': true, '?': true, ',': true, '.': true, ':': true, ';': true,
		'」': true, '』': true, ')': true, '）': true, ']': true, '］': true,
		'}': true, '｝': true, '〉': true, '》': true,
		'ぁ': true, 'ぃ': true, 'ぅ': true, 'ぇ': true, 'ぉ': true,
		'っ': true, 'ゃ': true, 'ゅ': true, 'ょ': true, 'ゎ': true,
		'ゕ': true, 'ゖ': true,
		'ァ': true, 'ィ': true, 'ゥ': true, 'ェ': true, 'ォ': true,
		'ッ': true, 'ャ': true, 'ュ': true, 'ョ': true, 'ヮ': true,
		'ｧ': true, 'ｨ': true, 'ｩ': true, 'ｪ': true, 'ｫ': true,
		'ｬ': true, 'ｭ': true, 'ｮ': true, 'ｯ': true,
		'ゝ': true, 'ゞ': true, 'ヽ': true, 'ヾ': true, '々': true,
		'ー': true, '・': true, 'ﾞ': true, 'ﾟ': true,
		'｡': true, '､': true, '･': true,
	}
	// Runes that must not end a line.
	kinsokuCannotEnd = map[rune]bool{
		'「': true, '『': true, '（': true, '(': true, '[': true, '{': true,
		'［': true, '｛': true, '〈': true, '《': true, '【': true,
		'〖': true, '〔': true, '"': true, '\'': true, '“': true,
		'‘': true, '＄': true, '$': true, '#': true, '&': true, '@': true,
	}
)

// WrapCueLines splits cue text into display lines for the target language.
// It never produces empty lines and always returns at least one line for
// non-empty input.
func WrapCueLines(text string, targetLang *string) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}

	lang := ""
	if targetLang != nil {
		lang = *targetLang
	}

	if lang != "" && isCJKLanguage(lang) || isCJKDominant(text) {
		return wrapCJK(text, lineColumns(lang))
	}
	return wrapLatin(text, defaultLatinLineColumns)
}

// isCJKDominant requires a clear majority of wide-glyph columns before
// applying CJK wrapping, so a stray full-width rune in an English cue does
// not flip it into space-stripping CJK mode.
func isCJKDominant(s string) bool {
	wide, total := 0, 0
	for _, r := range s {
		if unicode.IsSpace(r) {
			continue
		}
		total += cjkAwareWidth(r)
		if isWideRune(r) {
			wide += 2
		}
	}
	return total > 0 && wide*10 > total*7
}

func cjkAwareWidth(r rune) int {
	if isWideRune(r) {
		return 2
	}
	return 1
}

func lineColumns(targetLang string) int {
	if targetLang != "" && isCJKLanguage(targetLang) {
		return cjkLineColumns
	}
	return defaultLatinLineColumns
}

// wrapLatin wraps word-delimited text to the column limit. When two lines
// suffice it balances them with a bottom-heavy pyramid; otherwise it fills
// greedily.
func wrapLatin(text string, cols int) []string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return nil
	}

	widths := make([]int, len(words))
	for i, w := range words {
		widths[i] = textWidth(w)
	}

	// Single line fits?
	if total := sumWithSpaces(widths); total <= cols {
		return []string{strings.Join(words, " ")}
	}

	// Balanced two lines: prefer first line shorter or equal to second.
	best := -1
	bestImbalance := -1
	prefix := 0
	for i := 0; i < len(words)-1; i++ {
		prefix += widths[i]
		first := prefix + i // i spaces inside first line
		rest := totalWithoutWords(widths, i+1)
		if first <= cols && rest <= cols {
			imbalance := rest - first // second minus first; >= 0 preferred
			if imbalance < 0 {
				imbalance = -imbalance * 4 // penalize top-heavy strongly
			}
			if best == -1 || imbalance < bestImbalance {
				best, bestImbalance = i+1, imbalance
			}
		}
	}
	if best != -1 {
		return []string{
			strings.Join(words[:best], " "),
			strings.Join(words[best:], " "),
		}
	}

	return greedyWrapWords(words, widths, cols)
}

func greedyWrapWords(words []string, widths []int, cols int) []string {
	var lines []string
	var current []string
	currentWidth := 0

	for i, w := range words {
		wid := widths[i]

		// A single token wider than the line gets hard-broken so no line
		// silently exceeds the limit.
		for wid > cols {
			if len(current) > 0 {
				lines = append(lines, strings.Join(current, " "))
				current, currentWidth = nil, 0
			}
			remaining := w
			take := cols
			for textWidth(remaining) > take && len([]rune(remaining)) > 1 {
				cut := len([]rune(remaining))
				for cut > 1 && textWidth(string([]rune(remaining)[:cut])) > take {
					cut--
				}
				lines = append(lines, string([]rune(remaining)[:cut]))
				remaining = string([]rune(remaining)[cut:])
			}
			w = remaining
			wid = textWidth(w)
			if wid <= cols {
				break
			}
		}

		added := wid
		if len(current) > 0 {
			added++ // space
		}
		if len(current) > 0 && currentWidth+added > cols {
			lines = append(lines, strings.Join(current, " "))
			current, currentWidth = []string{w}, wid
			continue
		}
		current = append(current, w)
		currentWidth += added
	}
	if len(current) > 0 {
		lines = append(lines, strings.Join(current, " "))
	}
	return lines
}

// wrapCJK breaks CJK text at valid opportunities honoring kinsoku shori:
// never start a line with closing punctuation/small kana, never end one with
// opening brackets. It prefers breaking after delimiters near the midpoint.
func wrapCJK(text string, cols int) []string {
	runes := []rune(strings.Join(strings.Fields(text), ""))
	if len(runes) == 0 {
		return nil
	}

	var lines []string
	start := 0
	for start < len(runes) {
		end := fitEnd(runes, start, cols)
		lines = append(lines, string(runes[start:end]))
		start = end
	}
	return trimLines(lines)
}

// fitEnd finds the largest end index in (start, len] such that
// width(runes[start:end]) <= cols and no kinsoku rule is violated at the
// break. Falls back to a hard break when a single glyph exceeds cols.
func fitEnd(runes []rune, start, cols int) int {
	width := 0
	lastValidBreak := -1
	lastHardFit := start + 1

	for i := start; i < len(runes); i++ {
		w := cjkAwareWidth(runes[i])
		if width+w > cols {
			break
		}
		width += w
		lastHardFit = i + 1

		next := i + 1
		if next < len(runes) {
			cur := runes[i]
			nxt := runes[next]
			if !kinsokuCannotStart[nxt] && !kinsokuCannotEnd[cur] {
				lastValidBreak = next
			}
		} else {
			lastValidBreak = len(runes)
		}
	}

	if lastValidBreak > start {
		return lastValidBreak
	}
	return lastHardFit
}

func trimLines(lines []string) []string {
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}

func sumWithSpaces(widths []int) int {
	total := 0
	for _, w := range widths {
		total += w
	}
	if len(widths) > 1 {
		total += len(widths) - 1
	}
	return total
}

func totalWithoutWords(widths []int, from int) int {
	total := 0
	count := 0
	for i := from; i < len(widths); i++ {
		total += widths[i]
		count++
	}
	if count > 1 {
		total += count - 1
	}
	return total
}

// CueTextUnits estimates displayed length for reading-speed math: CJK glyphs
// count as one unit each, latin as one per character including spaces.
func CueTextUnits(text string, targetLang string) float64 {
	if targetLang != "" && isCJKLanguage(targetLang) {
		units := 0
		for _, r := range text {
			if unicode.IsSpace(r) {
				continue
			}
			units++
		}
		return float64(units)
	}
	return float64(len([]rune(text)))
}
