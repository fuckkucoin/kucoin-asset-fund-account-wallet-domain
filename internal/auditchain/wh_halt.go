package auditchain

import "sync/atomic"

var exportHalted atomic.Bool

// HaltExport 关掉后 ExportAllowed 直接拒。巡检恢复前由运维显式打开。

  func HaltExport(on bool) { exportHalted.Store(on) }

// ExportHalted 当前是否停了信任导出。

   func ExportHalted() bool { return exportHalted.Load() }
