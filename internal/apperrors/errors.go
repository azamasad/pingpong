package apperrors

import "net/http"

type APIError struct {
	Code       string
	Message    string
	Retryable  bool
	HTTPStatus int
}

func (e APIError) Error() string { return e.Message }

var status = map[string]int{
	"BAD_REQUEST":         400,
	"HOST_NOT_ALLOWED":    403,
	"ORIGIN_NOT_ALLOWED":  403,
	"NOT_FOUND":           404,
	"BODY_TOO_LARGE":      413,
	"MACHINE_UNKNOWN":     404,
	"MODEL_NOT_INSTALLED": 404,
	"MODEL_AMBIGUOUS":     409,
	"MODELS_UNKNOWN":      503,
	"NO_TARGET":           503,
	"MACHINE_DOWN":        503,
	"MACHINE_FAILED":      502,
	"TIMEOUT":             504,
	"UPSTREAM_ERROR":      502,
}

var retryable = map[string]bool{
	"MODELS_UNKNOWN": true,
	"MACHINE_DOWN":   true,
	"MACHINE_FAILED": true,
	"TIMEOUT":        true,
	"UPSTREAM_ERROR": true,
}

func New(code, message string) APIError {
	st, ok := status[code]
	if !ok {
		st = http.StatusInternalServerError
	}
	return APIError{
		Code:       code,
		Message:    message,
		Retryable:  retryable[code],
		HTTPStatus: st,
	}
}

func HTTPStatus(code string) int {
	if st, ok := status[code]; ok {
		return st
	}
	return 500
}
