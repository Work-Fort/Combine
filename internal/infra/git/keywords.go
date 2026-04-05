package git

import (
	"regexp"
	"strconv"
)

// closingKeywordRe matches "closes #N", "fixes #N", "resolves #N" and variants.
var closingKeywordRe = regexp.MustCompile(`(?i)\b(?:close[sd]?|fix(?:e[sd])?|resolve[sd]?)\s+#(\d+)`)

// ParseClosingKeywords extracts issue numbers referenced by closing keywords in text.
func ParseClosingKeywords(text string) []int64 {
	matches := closingKeywordRe.FindAllStringSubmatch(text, -1)
	seen := make(map[int64]bool)
	var nums []int64
	for _, m := range matches {
		if len(m) > 1 {
			n, err := strconv.ParseInt(m[1], 10, 64)
			if err == nil && !seen[n] {
				seen[n] = true
				nums = append(nums, n)
			}
		}
	}
	return nums
}
