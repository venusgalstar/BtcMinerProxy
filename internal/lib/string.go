package lib

import (
	"regexp"
)

var filenameSanitize = regexp.MustCompile(`([^a-zA-Z0-9.\+ ]+)`)

func SanitizeFilename(src string) string {
	return filenameSanitize.ReplaceAllLiteralString(src, "-")
}
