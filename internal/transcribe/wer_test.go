package transcribe

import "testing"

func TestComputeWER(t *testing.T) {
	tests := []struct {
		name       string
		reference  string
		hypothesis string
		wantWER    float64
		wantSubs   int
		wantIns    int
		wantDels   int
		wantRef    int
	}{
		{
			name:       "identical",
			reference:  "the cat sat on the mat",
			hypothesis: "the cat sat on the mat",
			wantWER:    0.0,
			wantRef:    6,
		},
		{
			name:       "one_substitution",
			reference:  "the cat sat on the mat",
			hypothesis: "the cat sit on the mat",
			wantWER:    1.0 / 6.0,
			wantSubs:   1,
			wantRef:    6,
		},
		{
			name:       "one_insertion",
			reference:  "the cat sat",
			hypothesis: "the big cat sat",
			wantWER:    1.0 / 3.0,
			wantIns:    1,
			wantRef:    3,
		},
		{
			name:       "one_deletion",
			reference:  "the cat sat on the mat",
			hypothesis: "the cat on the mat",
			wantWER:    1.0 / 6.0,
			wantDels:   1,
			wantRef:    6,
		},
		{
			name:       "case_insensitive",
			reference:  "The Cat Sat",
			hypothesis: "the cat sat",
			wantWER:    0.0,
			wantRef:    3,
		},
		{
			name:       "punctuation_stripped",
			reference:  "Hello, world!",
			hypothesis: "hello world",
			wantWER:    0.0,
			wantRef:    2,
		},
		{
			name:       "empty_reference",
			reference:  "",
			hypothesis: "some words",
			wantWER:    0.0,
			wantRef:    0,
		},
		{
			name:       "empty_hypothesis",
			reference:  "some words",
			hypothesis: "",
			wantWER:    1.0,
			wantDels:   2,
			wantRef:    2,
		},
		{
			name:       "both_empty",
			reference:  "",
			hypothesis: "",
			wantWER:    0.0,
			wantRef:    0,
		},
		{
			name:       "completely_different",
			reference:  "the cat sat",
			hypothesis: "a dog ran",
			wantWER:    1.0,
			wantSubs:   3,
			wantRef:    3,
		},
		{
			name:       "extra_whitespace",
			reference:  "  the   cat  sat  ",
			hypothesis: "the cat sat",
			wantWER:    0.0,
			wantRef:    3,
		},
		{
			name:       "mixed_errors",
			reference:  "the quick brown fox jumps over the lazy dog",
			hypothesis: "a quick brown cat jumps the lazy dog",
			// ref: the quick brown fox jumps over the lazy dog (9 words)
			// hyp: a   quick brown cat jumps       the lazy dog
			// sub: the->a, fox->cat = 2 subs; del: over = 1 del
			wantWER:  3.0 / 9.0,
			wantSubs: 2,
			wantDels: 1,
			wantRef:  9,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ComputeWER(tt.reference, tt.hypothesis)

			if diff := got.WER - tt.wantWER; diff > 0.001 || diff < -0.001 {
				t.Errorf("WER = %f, want %f", got.WER, tt.wantWER)
			}
			if got.RefWords != tt.wantRef {
				t.Errorf("RefWords = %d, want %d", got.RefWords, tt.wantRef)
			}
			if tt.wantSubs != 0 && got.Substitutions != tt.wantSubs {
				t.Errorf("Substitutions = %d, want %d", got.Substitutions, tt.wantSubs)
			}
			if tt.wantIns != 0 && got.Insertions != tt.wantIns {
				t.Errorf("Insertions = %d, want %d", got.Insertions, tt.wantIns)
			}
			if tt.wantDels != 0 && got.Deletions != tt.wantDels {
				t.Errorf("Deletions = %d, want %d", got.Deletions, tt.wantDels)
			}
		})
	}
}

func TestComputeWERResult(t *testing.T) {
	// Verify the full result struct for a known case
	got := ComputeWER(
		"ask not what your country can do for you",
		"ask what your country can do for you",
	)
	// "not" is deleted = 1 deletion out of 9 ref words
	if got.Deletions != 1 {
		t.Errorf("Deletions = %d, want 1", got.Deletions)
	}
	if got.RefWords != 9 {
		t.Errorf("RefWords = %d, want 9", got.RefWords)
	}
	wantWER := 1.0 / 9.0
	if diff := got.WER - wantWER; diff > 0.001 || diff < -0.001 {
		t.Errorf("WER = %f, want %f", got.WER, wantWER)
	}
}
