package occupbucketfence

import "errors"

// MutexConflict 将底层错误映射为互斥失败。

  func MutexConflict(err error) bool {
	  return errors.Is(err, ErrReclassMutex)
  }

// ConflictWinner 根据响应判定胜出腿。

  func ConflictWinner(resp *MutateResp, err error) string {
	  if resp == nil {
		   return ""
	   }
	   if err == nil {
		  return resp.Winner
	  }
	  if resp.Proof != nil && resp.Proof.WinnerOp != "" {
		  return resp.Proof.WinnerOp
	   }
	  return ""
   }
