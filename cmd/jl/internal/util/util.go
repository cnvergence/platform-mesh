package util

import (
	"fmt"
	"os"
	"strings"
)

func RemoveEmptyStrings(slice []string) []string {
	var result []string
	for _, s := range slice {
		if len(strings.TrimSpace(s)) > 0 {
			result = append(result, strings.TrimSpace(s))
		}
	}
	return result
}
func ContainString(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func PrintErrOut(msg string, err error) {
	_, printErr := fmt.Fprintln(os.Stderr, msg, err)
	if printErr != nil { // coverage-ignore
		// Fallback is to print to stdout instead
		fmt.Println(msg, err)
	}
}
