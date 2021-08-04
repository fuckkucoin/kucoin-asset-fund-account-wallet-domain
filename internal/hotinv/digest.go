package hotinv

// SnapshotKey 返回 Snapshot 的幂等键（asset:hotAvail:hotMax），
// 供 hotpregate 快速判断快照是否发生变化（精确核对使用 hotpregate.ComputeDigest）。

  func (s Snapshot) SnapshotKey() string {
	   return s.Asset + "|" + s.HotAvail + "|" + s.HotMax
  }
