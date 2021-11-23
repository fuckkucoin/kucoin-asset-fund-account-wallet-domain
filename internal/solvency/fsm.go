package solvency

// AllowAttest 报告 from 能否进入 to。同一状态不算迁移。
// building 只能封存或废弃；封存后才能发布；发布与封存都可以作废。
// 废弃与作废是终态。半截根留在 building 或 abandoned，不能进入 published。
   func AllowAttest(from, to Status) bool {
	  if from == "" || to == "" || from == to {
		   return false
	  }
	  switch from {
	   case StatusBuilding:
		  return to == StatusSealed || to == StatusAbandoned
	  case StatusSealed:
		   return to == StatusPublished || to == StatusRevoked
	   case StatusPublished:
		   return to == StatusRevoked
	  default:
		   return false
	   }
  }

// AllowDrift 短缺工单只能 open → acked → closed。

  func AllowDrift(from, to DriftStatus) bool {
	   if from == "" || to == "" || from == to {
		  return false
	   }
	  switch from {
	   case DriftOpen:
		  return to == DriftAcked
	  case DriftAcked:
		  return to == DriftClosed
	   default:
		   return false
	  }
  }

// TerminalAttest 终态不可再封存或发布。

  func TerminalAttest(s Status) bool {
	  return s == StatusRevoked || s == StatusAbandoned
  }

// ReadableProof 只有封存或已发布的根才能出具包含路径。building 的草稿根不能验证。
   func ReadableProof(s Status) bool {
	  return s == StatusSealed || s == StatusPublished
  }

  func deriveSolvency(class string) (solvency, deprecated bool) {
	  switch class {
	  case ClassInternal, ClassPublishable:
		   return true, false
	  default:
		   return false, true
	  }
   }
