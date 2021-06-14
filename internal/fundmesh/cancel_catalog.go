package fundmesh

// EntryParityRows 双入口必须同码的场景。
  func EntryParityRows() [][4]string {
	  return [][4]string{
		   {"frozen", "DELETE", "POST", CodeOK},
		   {"cancelled", "DELETE", "POST", CodeIdempotent},
		   {"broadcast", "DELETE", "POST", CodeBroadcast},
		  {"confirmed", "DELETE", "POST", CodeConfirmed},
		  {"failed", "DELETE", "POST", CodeFailed},
		  {"missing", "DELETE", "POST", CodeNotFound},
	   }
  }

  func SameCancelCode(a, b CancelResult) bool {
	   if a.Code != b.Code {
		  return false
	   }
	   if (a.State == CancelAccepted || a.State == CancelIdempotent) != (b.State == CancelAccepted || b.State == CancelIdempotent) {
		  return false
	   }
	   return true
   }

  func NormalizeReason(reason string) string {
	   if reason == "" {
		   return "cancel"
	   }
	   return reason
   }
