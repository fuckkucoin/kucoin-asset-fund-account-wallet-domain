// @FileName: outbound_policy.go
// @Summary: 出站幂等围栏策略。
// 重点：禁止同键二次扣减；与取消 fail-closed 共用关键写闸。
// 风险点：单独放开出站开关会绕过 cancel 同源闸。
// @Author: Blaise
// @Created: 2021.06.14

package fundmesh

  func (p Policy) OutboundFailClosed() bool {
	   return p.FailClosedOnDrift && p.RequireRemoteLedger
   }

   func RejectSecondDebit(rec *OutboundRecord) error {
	   if rec == nil {
		  return nil
	  }
	   if rec.DebitCount > 0 || !OutboundDebitAllowed(rec.State, rec.Phase) {
		  return ErrDoubleDebitForbidden
	  }
	   return nil
  }

  func OutboundUsesCancelGate() bool { return true }
