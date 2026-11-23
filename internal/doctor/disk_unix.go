//go:build unix

package doctor

import (
	"fmt"
	"syscall"
)

func freeBytesOnFS(path string) (uint64, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, err
	}
	if st.Bsize <= 0 {
		return 0, fmt.Errorf("statfs: invalid block size")
	}
	return uint64(st.Bavail) * uint64(st.Bsize), nil
}
