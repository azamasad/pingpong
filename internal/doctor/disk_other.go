//go:build !unix

package doctor

import "fmt"

func freeBytesOnFS(path string) (uint64, error) {
	return 0, fmt.Errorf("disk space check not supported on this platform")
}
