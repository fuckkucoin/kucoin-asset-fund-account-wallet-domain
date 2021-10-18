package recon

import (
	"context"
	"fmt"
	"time"
)

// Incident 对账未结差异触发的事故摘要。

type Incident struct {
	TicketID string `json:"ticket_id"`
	UserID string `json:"user_id"`
	OpenedBy string        `json:"opened_by"`
	Age      time.Duration `json:"age"`
	Engaged bool   `json:"engaged"`
	Note    string `json:"note,omitempty"`
}

// IncidentHooks 升级时可挂熔断；Clear 不在此路径。
type IncidentHooks struct {
	Halt HaltEngager
}

// RaiseIncidents 扫描超 SLA 工单；默认 auto 作用域、MinLevel=2 才 Engage。
// 把对账差异抬升为运营事故信号。Engage 失败不得假装成功；不得 Clear。误 Engage 会停充提，须值班确认后走运营 Clear。
   func (w *Workflow) RaiseIncidents(ctx context.Context, now time.Time, actor string, hooks IncidentHooks) ([]Incident, error) {
	   return w.RaiseIncidentsPolicy(ctx, now, actor, hooks, IncidentPolicy{})
  }

// DepositWithdrawFence 事故未结时挡充提新单入口的说明性错误。
// 不修改 deposit/withdraw 内部幂等；由调用方在入口检查。
   func DepositWithdrawFence(w *Workflow, userID string) error {
	   if w == nil {
		  return nil
	   }
	  if w.BlocksUser(userID) {
		   return fmt.Errorf("reconcile open mismatch fences deposit/withdraw")
	   }
	  return nil
   }
