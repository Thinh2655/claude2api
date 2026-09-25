package utils

import "strings"

// EstimateTokens returns an approximate token count for a string: one word per
// token, split on whitespace. claude.ai never reports token counts, so a word
// count is the dashboard/usage heuristic.
func EstimateTokens(s string) int {
	return len(strings.Fields(s))
}

// EstimatePromptTokens counts the input side, which already carries system
// messages and role prefixes through ProcessMessages.
func EstimatePromptTokens(prompt string) int {
	return EstimateTokens(prompt)
}

// EstimateCompletionTokens counts the output side from the streamed assistant
// text, including the thinking and code-fence decorations HandleResponse adds.
func EstimateCompletionTokens(text string) int {
	return EstimateTokens(text)
}
