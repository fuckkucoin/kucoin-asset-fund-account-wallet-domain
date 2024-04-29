package airgap

import "sync"

var (
	procOnce sync.Once
	procSvc  *Service
)

// Process 返回进程内气隙服务。

  func Process() *Service {
	   procOnce.Do(func() { procSvc = New() })
	  return procSvc
   }
