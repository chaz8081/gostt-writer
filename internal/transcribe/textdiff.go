package transcribe

// computeDelta calculates the minimal edit from prevText to newText as a
// number of backspaces (to delete divergent suffix of prev) and an append
// string (new characters after the common prefix).
//
// Common case (pure append): backspaces=0, appendText=new suffix.
// Correction case: backspaces>0 when the sliding window revised earlier text.
func computeDelta(prevText, newText string) (backspaces int, appendText string) {
	prevRunes := []rune(prevText)
	newRunes := []rune(newText)

	// Find longest common prefix (by rune for Unicode safety)
	minLen := len(prevRunes)
	if len(newRunes) < minLen {
		minLen = len(newRunes)
	}

	commonLen := 0
	for i := 0; i < minLen; i++ {
		if prevRunes[i] != newRunes[i] {
			break
		}
		commonLen++
	}

	backspaces = len(prevRunes) - commonLen
	appendText = string(newRunes[commonLen:])
	return
}
