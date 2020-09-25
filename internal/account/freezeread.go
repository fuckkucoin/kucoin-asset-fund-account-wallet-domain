// 按用户取出控制位审计和科目闸审计，并标明当前用户是否仍冻。科目记录的实体是 user|book，不能和用户控制位混成一条。只读用户控制位会漏掉科目仍冻；新的冻结仍会被科目闸拦住。

// Author: CainReed
// Date: 2020.09.17

package account

import (
	"context"
	"strings"
	"account-assets.kucoin.net/funding/internal/ledger"
)

// FreezeRead 用户控制位与科目闸的当前读数。
type FreezeRead struct {
	UserID string `json:"user_id"`
	Frozen bool   `json:"frozen"`
	User []ledger.AuditEvent `json:"user_audits,omitempty"`
	Subject []ledger.AuditEvent `json:"subject_audits,omitempty"`
}

// FreezeReadOf 合并两类审计。不改冻结位。

   func (s *Service) FreezeReadOf(ctx context.Context, userID string, limit int) (FreezeRead, error) {
	   out := FreezeRead{}
	  if s == nil || s.led == nil {
		   return out, ledger.ErrNotFound
	  }
	  userID = strings.TrimSpace(userID)
	   if userID == "" {
		   return out, ledger.ErrInvalidAddress
	  }
	  if limit <= 0 || limit > 200 {
		  limit = 50
	  }
	  on, err := s.led.Frozen(ctx, userID)
	   if err != nil {
		  return out, err
	   }
	   userEv, err := s.led.ListAudit(ctx, "user", userID, limit)
	  if err != nil {
		   return out, err
	   }
	  subEv, err := s.SubjectAudits(ctx, userID, limit)
	  if err != nil {
		  return out, err
	   }
	   out.UserID = userID
	  out.Frozen = on
	   out.User = filterUserFreeze(userEv)
	  out.Subject = subEv
	  return out, nil
   }

   func filterUserFreeze(in []ledger.AuditEvent) []ledger.AuditEvent {
	  if len(in) == 0 {
		   return nil
	   }
	  out := make([]ledger.AuditEvent, 0, len(in))
	  for _, e := range in {
		   switch e.Action {
		  case "control_frozen", "user_unfreeze_rejected", "security_change":
			  out = append(out, e)
		   }
	   }
	  return out
   }
