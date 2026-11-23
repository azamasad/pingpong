package api

import (
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/azamasad/pingpong/internal/apperrors"
)

func checkHost(r *http.Request, allowed []string) error {
	host := r.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.ToLower(host)
	for _, a := range allowed {
		if strings.ToLower(a) == host {
			return nil
		}
	}
	return apperrors.New("HOST_NOT_ALLOWED", "host not allowed")
}

func checkOrigin(r *http.Request, allowed []string) error {
	if r.Method != http.MethodPost {
		return nil
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		return nil
	}
	if origin == "null" {
		return apperrors.New("ORIGIN_NOT_ALLOWED", "origin not allowed")
	}
	u, err := url.Parse(origin)
	if err != nil {
		return apperrors.New("ORIGIN_NOT_ALLOWED", "origin not allowed")
	}
	host := strings.ToLower(u.Hostname())
	for _, a := range allowed {
		if strings.ToLower(a) == host {
			return nil
		}
	}
	return apperrors.New("ORIGIN_NOT_ALLOWED", "origin not allowed")
}
