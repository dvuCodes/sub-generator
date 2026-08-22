package main

import (
	"strings"
	"testing"
)

func eqLines(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func TestWrapLatinBalancesTwoLinesBottomHeavy(t *testing.T) {
	text := "The quick brown fox jumps over the lazy dog near the riverbank every morning"
	lines := wrapLatin(text, 42)
	if len(lines) != 2 {
		t.Fatalf("wrapLatin() lines = %d, want 2: %#v", len(lines), lines)
	}
	first, second := textWidth(lines[0]), textWidth(lines[1])
	if first > 42 || second > 42 {
		t.Fatalf("lines exceed column limit: %d, %d", first, second)
	}
	if first > second {
		t.Fatalf("expected bottom-heavy pyramid: first=%d second=%d", first, second)
	}
}

func TestWrapLatinSingleLineFits(t *testing.T) {
	lines := wrapLatin("Hello world", 42)
	if !eqLines(lines, []string{"Hello world"}) {
		t.Fatalf("wrapLatin() = %#v", lines)
	}
}

func TestWrapLatinHardBreaksOversizedToken(t *testing.T) {
	token := "Supercalifragilisticexpialidociousandthensomemoreletters"
	lines := greedyWrapWords([]string{token}, []int{textWidth(token)}, 20)
	if len(lines) < 2 {
		t.Fatalf("expected hard break into multiple lines, got %#v", lines)
	}
	for i, line := range lines {
		if textWidth(line) > 20 {
			t.Fatalf("line %d exceeds limit: %q (%d cols)", i, line, textWidth(line))
		}
	}
	if rejoined := strings.Join(lines, ""); rejoined != token {
		t.Fatalf("hard break lost content: %q != %q", rejoined, token)
	}
}

func TestWrapCJKHonorsColumnLimit(t *testing.T) {
	text := "これはテスト用の日本語字幕です。各行が正しい幅に収まることを確認します。"
	lines := wrapCJK(text, cjkLineColumns)
	if len(lines) < 2 {
		t.Fatalf("wrapCJK() produced %d lines, want >= 2", len(lines))
	}
	for i, line := range lines {
		if w := textWidth(line); w > cjkLineColumns {
			t.Fatalf("line %d width %d exceeds %d: %q", i, w, cjkLineColumns, line)
		}
		if line == "" {
			t.Fatalf("empty line emitted: %#v", lines)
		}
	}
}

func TestWrapCJKKinsokuNeverStartsLineWithClosingPunctuation(t *testing.T) {
	text := "前半のセリフです。後半のセリフが続きます。さらに別の文もここに入ります。"
	runes := []rune(text)

	lines := wrapCJK(text, 13)
	offset := 0
	for _, line := range lines {
		lineRunes := []rune(line)
		if len(lineRunes) == 0 {
			continue
		}
		// First rune of continuation lines must not be a forbidden starter.
		if offset > 0 && kinsokuCannotStart[lineRunes[0]] {
			t.Fatalf("line starts with forbidden glyph %q: %q", lineRunes[0], line)
		}
		last := lineRunes[len(lineRunes)-1]
		if kinsokuCannotEnd[last] && offset+len(lineRunes) < len(runes) {
			t.Fatalf("line ends with forbidden opener %q: %q", last, line)
		}
		offset += len(lineRunes)
	}
	if offset != len(runes) {
		t.Fatalf("kinsoku wrapping dropped content: covered %d of %d runes", offset, len(runes))
	}
}

func TestWrapCueLinesKeepsLatinSpacesInMixedText(t *testing.T) {
	text := "This is English with one wide mark！ Keep spacing intact."
	lines := WrapCueLines(text, strPtr("en"))
	joined := ""
	for i, l := range lines {
		if i > 0 {
			joined += "\n"
		}
		joined += l
	}
	if !containsWord(joined, "Keep") || !containsWord(joined, "spacing") {
		t.Fatalf("mixed-script cue mangled: %q", joined)
	}
}

func TestWrapCueLinesUsesCJKModeForJapaneseTarget(t *testing.T) {
	text := "日本語の字幕は十三文字ずつで改行されるべきです。"
	lines := WrapCueLines(text, strPtr("ja"))
	if len(lines) < 2 {
		t.Fatalf("expected multiple lines for long JA cue, got %#v", lines)
	}
	for _, line := range lines {
		if textWidth(line) > cjkLineColumns {
			t.Fatalf("JA line too wide: %q (%d)", line, textWidth(line))
		}
	}
}

func TestIsCJKDominant(t *testing.T) {
	if isCJKDominant("Totally english sentence") {
		t.Fatal("english should not be CJK dominant")
	}
	if !isCJKDominant("これは日本語です") {
		t.Fatal("japanese should be CJK dominant")
	}
	if isCJKDominant("English with one ！ mark") {
		t.Fatal("single wide rune should not flip mode")
	}
}

func containsWord(haystack, word string) bool {
	for i := 0; i+len(word) <= len(haystack); i++ {
		if haystack[i:i+len(word)] == word {
			return true
		}
	}
	return false
}
