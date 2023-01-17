package wdrelease

// WritePath 释放台写路径标识。
type WritePath string

const (
	PathAdmit        WritePath = "admit"
	PathBeforeFreeze WritePath = "before_freeze"
	PathCommitFreeze WritePath = "commit_freeze"
	PathAbortFreeze WritePath = "abort_freeze"
	PathExpire      WritePath = "expire"
	PathReopen WritePath = "reopen"
	PathLiveUnfreeze WritePath = "live_unfreeze"
)

// LiveUnfreezePaths 可能被误认为解冻的路径。
// AbortFreeze 仅退 reserved 占用，不是活链解冻。
   func LiveUnfreezePaths() []WritePath {
	   return []WritePath{PathLiveUnfreeze}
   }

// IsLiveUnfreezePath 是否活链解冻旁路。

  func IsLiveUnfreezePath(p WritePath) bool {
	  return p == PathLiveUnfreeze
   }

// PathAllowsUnfreeze 必须对所有路径返回 false。
  func PathAllowsUnfreeze(p WritePath) bool {
	   _ = p
	  return false
  }

// ExportRefuseMap 运维只读：路径 → 是否允许解冻。
   func ExportRefuseMap() map[WritePath]bool {
	  return map[WritePath]bool{
		   PathAdmit:        false,
		   PathBeforeFreeze: false,
		  PathCommitFreeze: false,
		  PathAbortFreeze: false,
		  PathExpire: false,
		   PathReopen: false,
		   PathLiveUnfreeze: false,
	  }
  }
