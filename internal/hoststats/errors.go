package hoststats

import "errors"

var (
	errListen      = errors.New("listen address must be an IP or localhost")
	errUnspecified = errors.New("refusing to listen on all interfaces")
)
