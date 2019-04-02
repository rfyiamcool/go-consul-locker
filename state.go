package consulocker

import (
	"sync/atomic"
)

var (
	isLocked  int32 = 1
	notLocked int32 = 0
)

func SetLockFlag(n *int32) {
	atomic.StoreInt32(n, isLocked)
}

func IsLocked(n *int32) bool {
	v := atomic.LoadInt32(n)
	if v == isLocked {
		return true
	}

	return false
}
