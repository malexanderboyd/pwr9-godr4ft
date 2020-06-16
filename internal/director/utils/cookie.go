package utils

import (
	"net/http"
	"time"
)

func CreateDraftClientIDCookieHeader(clientID, cookieName string) http.Header {
	var clientIDHeader = http.Header{}
	clientIdCookie := &http.Cookie{
		Name:    cookieName,
		Value:   clientID,
		Path:    "/",
		Expires: time.Now().Add(time.Minute * 30),
	}
	if v := clientIdCookie.String(); v != "" {
		clientIDHeader.Add("Set-Cookie", v)
	}
	return clientIDHeader
}

func HasDraftClientIDCookie(r *http.Request, cookieName string) (bool, string) {
	cookies := r.Cookies()
	for _, cookie := range cookies {
		if cookie.Name == cookieName {
			return true, cookie.Value
		}
	}
	return false, ""
}
