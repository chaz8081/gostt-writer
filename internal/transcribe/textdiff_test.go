package transcribe

import "testing"

func TestComputeDelta(t *testing.T) {
	tests := []struct {
		name           string
		prev, new      string
		wantBackspaces int
		wantAppend     string
	}{
		{
			name:           "empty to text",
			prev:           "",
			new:            "hello",
			wantBackspaces: 0,
			wantAppend:     "hello",
		},
		{
			name:           "pure append",
			prev:           "hello",
			new:            "hello world",
			wantBackspaces: 0,
			wantAppend:     " world",
		},
		{
			name:           "correction",
			prev:           "hello word",
			new:            "hello world",
			wantBackspaces: 1,
			wantAppend:     "ld",
		},
		{
			name:           "complete replacement",
			prev:           "abc",
			new:            "xyz",
			wantBackspaces: 3,
			wantAppend:     "xyz",
		},
		{
			name:           "text to empty",
			prev:           "hello",
			new:            "",
			wantBackspaces: 5,
			wantAppend:     "",
		},
		{
			name:           "both empty",
			prev:           "",
			new:            "",
			wantBackspaces: 0,
			wantAppend:     "",
		},
		{
			name:           "identical",
			prev:           "hello",
			new:            "hello",
			wantBackspaces: 0,
			wantAppend:     "",
		},
		{
			name:           "unicode append",
			prev:           "café",
			new:            "café latte",
			wantBackspaces: 0,
			wantAppend:     " latte",
		},
		{
			name:           "unicode pure append",
			prev:           "café",
			new:            "cafétéria",
			wantBackspaces: 0,
			wantAppend:     "téria",
		},
		{
			name:           "unicode correction",
			prev:           "naïve",
			new:            "naïvety",
			wantBackspaces: 0,
			wantAppend:     "ty",
		},
		{
			name:           "shorter replacement",
			prev:           "hello world",
			new:            "hello",
			wantBackspaces: 6,
			wantAppend:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backspaces, appendText := computeDelta(tt.prev, tt.new)
			if backspaces != tt.wantBackspaces {
				t.Errorf("backspaces = %d, want %d", backspaces, tt.wantBackspaces)
			}
			if appendText != tt.wantAppend {
				t.Errorf("appendText = %q, want %q", appendText, tt.wantAppend)
			}
		})
	}
}
