package jwt

import (
	"net/http"
	"regexp"
)

var spiffeUriReg = regexp.MustCompile(`URI=([a-z:\/\/\.\-]*)`)

func GetSpiffeUrlValue(header http.Header) *string {
	headervalue := header.Get(HeaderSpiffeValue)
	uriVal := GetURIValue(headervalue)

	if len(uriVal) > 0 {
		return &uriVal
	}
	return nil
}

func GetURIValue(headerVal string) string {
	match := spiffeUriReg.FindSubmatch([]byte(headerVal))
	if len(match) == 2 {
		return string(match[1])
	} else {
		return ""
	}
}
