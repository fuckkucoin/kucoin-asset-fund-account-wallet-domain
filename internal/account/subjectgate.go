// 科目冻结写 subject_frozen；用户仍冻时拒绝解科目；带连带标记的解冻直接拒绝。本闸不调用用户 SetFrozen，避免一次操作同时改两套冻结。若解科目时不看用户控制位，用户冻会被科目解冻悄悄放行。

// Author: Draven
// Date: 2021.01.03

package account

import (
	"context"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/ledger"
)

// FreezeGated 科目冻解冻。releaseOthers 为真时拒绝，科目解冻不会放开其它科目或用户控制位。

  func (s *Service) FreezeGated(ctx context.Context, userID string, book Book, frozen, releaseOthers bool) (BookState, error) {
	   if s == nil {
		   return BookState{}, ledger.ErrNotFound
	  }
	   userID = strings.TrimSpace(userID)
	  if userID == "" {
		  return BookState{}, ledger.ErrInvalidAddress
	   }
	   if !frozen && releaseOthers {
		  s.writeSubjectAudit(ctx, userID, book, "subject_unfreeze_rejected", "subject unfreeze does not release other freezes")
		   return BookState{}, ledger.ErrFreezeGate
	  }
	   if !frozen && s.led != nil {
		  on, err := s.led.Frozen(ctx, userID)
		  if err != nil {
			   return BookState{}, err
		   }
		  if on {
			   s.writeSubjectAudit(ctx, userID, book, "subject_unfreeze_rejected", "user freeze gate still closed")
			   return BookState{}, ErrBookFrozen
		   }
	  }
	  st, err := s.Freeze(ctx, userID, book, frozen)
	   if err != nil {
		  return st, err
	   }
	  action := "subject_frozen"
	   if !frozen {
		   action = "subject_unfrozen"
	  }
	  s.writeSubjectAudit(ctx, userID, book, action, st.Reason)
	  return st, nil
  }

// SubjectAudits 科目闸审计。userID 为空时返回最近的科目记录。

  func (s *Service) SubjectAudits(ctx context.Context, userID string, limit int) ([]ledger.AuditEvent, error) {
	  if s == nil || s.led == nil {
		  return nil, ledger.ErrNotFound
	  }
	  if limit <= 0 || limit > 200 {
		   limit = 50
	   }
	  evs, err := s.led.ListAudit(ctx, "subject", "", 200)
	   if err != nil {
		  return nil, err
	  }
	   userID = strings.TrimSpace(userID)
	   out := make([]ledger.AuditEvent, 0, limit)
	  for _, e := range evs {
		  if userID != "" && !subjectAuditFor(e, userID) {
			   continue
		   }
		   out = append(out, e)
		   if len(out) >= limit {
			   break
		  }
	   }
	   return out, nil
   }

   func subjectAuditFor(e ledger.AuditEvent, userID string) bool {
	  if e.EntityID == userID || strings.HasPrefix(e.EntityID, userID+"|") {
		   return true
	  }
	   if e.Detail == nil {
		  return false
	   }
	  got, _ := e.Detail["user_id"].(string)
	   return got == userID
   }

   func (s *Service) writeSubjectAudit(ctx context.Context, userID string, book Book, action, reason string) {
	   if s == nil || s.led == nil || s.led.Store() == nil {
		  return
	   }
	  _ = s.led.Store().AppendAudit(ctx, ledger.AuditEvent{
		   EntityType: "subject",
		  EntityID:   userID + "|" + string(book),
		   Action: action,
		   Actor: "ops",
		  Detail: map[string]any{
			   "gate": "subject", "user_id": userID, "book": string(book), "reason": reason,
		   },
		  CreatedAt: time.Now().UTC(),
	   })
   }
