//go:build !linux && !darwin

package hoststats

import "errors"

func readHost() (raw, error) {
	return raw{}, errors.New("unsupported")
}
