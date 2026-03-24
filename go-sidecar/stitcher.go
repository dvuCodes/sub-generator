package main

// ContextBlock groups consecutive segments whose inter-segment silence
// is below a configurable threshold. Blocks provide translation context
// but never alter original segment timestamps.
type ContextBlock struct {
	Segments []Segment
	Start    float64 // = Segments[0].Start
	End      float64 // = Segments[len-1].End
}

// StitcherConfig holds tunable knobs for the segment grouping algorithm.
type StitcherConfig struct {
	GapThreshold        float64 // max silence (seconds) between segments to keep in same block
	MaxBlockLength      float64 // max time span (seconds) of a single block
	MaxSegmentsPerBlock int     // max number of segments per block
}

// DefaultStitcherConfig returns production defaults.
func DefaultStitcherConfig() StitcherConfig {
	return StitcherConfig{
		GapThreshold:        2.0,
		MaxBlockLength:      30.0,
		MaxSegmentsPerBlock: 10,
	}
}

// StitchSegments groups segments into context blocks for contextual translation.
// Segments are assumed pre-sorted by Start (as returned by whisper-server).
// Blank or near-blank segments are preserved in order and never dropped.
func StitchSegments(segments []Segment, cfg StitcherConfig) []ContextBlock {
	if len(segments) == 0 {
		return nil
	}

	var blocks []ContextBlock
	current := ContextBlock{
		Segments: []Segment{segments[0]},
		Start:    segments[0].Start,
		End:      segments[0].End,
	}

	for i := 1; i < len(segments); i++ {
		seg := segments[i]
		gap := seg.Start - current.End
		proposedSpan := seg.End - current.Start
		proposedCount := len(current.Segments) + 1

		if gap <= cfg.GapThreshold && proposedSpan <= cfg.MaxBlockLength && proposedCount <= cfg.MaxSegmentsPerBlock {
			current.Segments = append(current.Segments, seg)
			current.End = seg.End
		} else {
			blocks = append(blocks, current)
			current = ContextBlock{
				Segments: []Segment{seg},
				Start:    seg.Start,
				End:      seg.End,
			}
		}
	}

	blocks = append(blocks, current)
	return blocks
}
