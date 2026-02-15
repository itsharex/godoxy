package icons

import (
	"sync/atomic"

	"github.com/yusing/godoxy/internal/common"
)

type Provider interface {
	HasIcon(u *URL) bool
}

var provider atomic.Value

func SetProvider(p Provider) {
	provider.Store(p)
}

func hasIcon(u *URL) bool {
	if common.IsTest {
		return true
	}
	v := provider.Load()
	if v == nil {
		return false
	}
	return v.(Provider).HasIcon(u)
}
