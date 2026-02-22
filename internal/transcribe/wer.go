package transcribe

import (
	"strings"
	"unicode"
)

// WERResult holds detailed word error rate results.
type WERResult struct {
	WER           float64 // Word Error Rate (0.0 = perfect, 1.0+ = very bad)
	Substitutions int     // Words replaced with different words
	Insertions    int     // Extra words in hypothesis
	Deletions     int     // Words missing from hypothesis
	RefWords      int     // Total words in reference
}

// ComputeWER calculates the word error rate between reference and hypothesis text.
// Both strings are normalized: lowercased, punctuation stripped, whitespace collapsed.
// WER = (Substitutions + Insertions + Deletions) / ReferenceWordCount.
func ComputeWER(reference, hypothesis string) WERResult {
	refWords := normalizeWords(reference)
	hypWords := normalizeWords(hypothesis)

	n := len(refWords)
	if n == 0 {
		return WERResult{}
	}

	m := len(hypWords)

	// DP table for minimum edit distance.
	d := make([][]int, n+1)
	for i := range d {
		d[i] = make([]int, m+1)
		d[i][0] = i // deleting all ref words
	}
	for j := 0; j <= m; j++ {
		d[0][j] = j // inserting all hyp words
	}

	for i := 1; i <= n; i++ {
		for j := 1; j <= m; j++ {
			if refWords[i-1] == hypWords[j-1] {
				d[i][j] = d[i-1][j-1]
			} else {
				sub := d[i-1][j-1] + 1
				del := d[i-1][j] + 1
				ins := d[i][j-1] + 1
				d[i][j] = min(sub, min(del, ins))
			}
		}
	}

	// Backtrace to count substitutions, insertions, deletions.
	var subs, ins, dels int
	i, j := n, m
	for i > 0 || j > 0 {
		if i > 0 && j > 0 && refWords[i-1] == hypWords[j-1] {
			// Match
			i--
			j--
		} else if i > 0 && j > 0 && d[i][j] == d[i-1][j-1]+1 {
			// Substitution
			subs++
			i--
			j--
		} else if i > 0 && d[i][j] == d[i-1][j]+1 {
			// Deletion (ref word missing from hyp)
			dels++
			i--
		} else {
			// Insertion (extra word in hyp)
			ins++
			j--
		}
	}

	return WERResult{
		WER:           float64(subs+ins+dels) / float64(n),
		Substitutions: subs,
		Insertions:    ins,
		Deletions:     dels,
		RefWords:      n,
	}
}

// normalizeWords lowercases text, strips punctuation, and splits into words.
func normalizeWords(s string) []string {
	s = strings.ToLower(s)
	s = strings.Map(func(r rune) rune {
		if unicode.IsPunct(r) {
			return -1
		}
		return r
	}, s)
	return strings.Fields(s)
}
