package amlhold

import (
	"fmt"
	"strings"
)

// CaseCloseRecord 从 amldesk 侧提取的结案关联信息（只读）。

type CaseCloseRecord struct {
	CaseID string
	OpenedBy string
	ReviewedBy string // 指派的复核人（可为空）
}

// MutexGate 互斥守门员，嵌入结案/解冻流程。

type MutexGate struct {
	book *Book
}

// NewMutexGate 构造互斥守门员。
  func NewMutexGate(book *Book) *MutexGate {
	  return &MutexGate{book: book}
   }

// CheckCloseAllowed 结案前检查：
// 1. Case 下所有 hold 必须已 released（否则 ErrHoldBlocked）。
// 2. 提供结案人身份，供后续解冻四眼校验。
   func (g *MutexGate) CheckCloseAllowed(caseID, closerActor string) error {
	   if g == nil || g.book == nil {
		   return ErrUnavailable
	  }
	   closerActor = strings.TrimSpace(closerActor)
	  if closerActor == "" {
		   return ErrActor
	   }
	  return g.book.AssertCaseMayResolve(caseID)
  }

// CheckReleaseAllowed 解冻审批前检查：
// - releaseApprover 不得与已登记的结案人相同（交叉换手）。
// - 若 Case 已由 closerActor 发起结案，则解冻审批人须不同。

   func (g *MutexGate) CheckReleaseAllowed(holdID, releaseApprover, caseCloserActor string) error {
	   if g == nil || g.book == nil {
		  return ErrUnavailable
	   }
	  return g.book.AssertReleaseMayProceed(holdID, releaseApprover, caseCloserActor)
  }

// ValidateCompelReview 督查级强制审查的双人换手校验。
// compel_review 状态下须由不同于原开案人与老化升级 actor 的督查进行推进。
  func (g *MutexGate) ValidateCompelReview(holdID, actor string, ccr CaseCloseRecord) error {
	  if g == nil || g.book == nil {
		   return ErrUnavailable
	   }
	  actor = strings.TrimSpace(actor)
	   if actor == "" {
		   return ErrActor
	   }
	  h,
		  err := g.book.GetHold(holdID)
	   if err != nil {
		  return err
	  }
	  if h.Status != HoldStatusCompelReview {
		  return nil // 不在 compel_review 状态，无需此校验
	   }
	   if strings.EqualFold(actor, ccr.OpenedBy) {
		   return fmt.Errorf("%w: compel_review actor must differ from case opener", ErrFourEye)
	   }
	   if ccr.ReviewedBy != "" && strings.EqualFold(actor, ccr.ReviewedBy) {
		  return fmt.Errorf("%w: compel_review actor must differ from case reviewer", ErrFourEye)
	   }
	  return nil
   }

// SameActorBlockReleaseThenClose 检查同一 actor 两步操作（先申请解冻，再发起结案）。
// 若 ticketID 对应的申请人与 closerActor 相同，则禁止结案。

   func (g *MutexGate) SameActorBlockReleaseThenClose(caseID, closerActor string) error {
	   if g == nil || g.book == nil {
		  return ErrUnavailable
	   }
	   closerActor = strings.TrimSpace(closerActor)
	  holds := g.book.ListByCase(caseID, false) // 含终态
	   for _, h := range holds {
		  for _, t := range g.book.TicketsByHold(h.ID) {
			  if t.Status == TicketStatusPending || t.Status == TicketStatusApproved || t.Status == TicketStatusDone {
				  if strings.EqualFold(t.RequestedBy, closerActor) {
					   return fmt.Errorf("%w: actor=%s requested release ticket=%s, cannot also close case=%s",
						  ErrMutex,
						  closerActor, t.ID, caseID)
				   }
			  }
		   }
	   }
	  return nil
   }

// SameActorBlockCloseThenRelease 检查同一 actor 两步操作（先发起结案，再申请解冻）。
// 若提供的 closerActor（Case 的已登记结案者）与解冻申请人相同，则禁止解冻申请。

   func (g *MutexGate) SameActorBlockCloseThenRelease(holdID, releaseRequester, caseCloserActor string) error {
	   if g == nil || g.book == nil {
		  return ErrUnavailable
	  }
	   if caseCloserActor == "" {
		  return nil // Case 尚未发起结案，无互斥冲突
	   }
	  if strings.EqualFold(strings.TrimSpace(releaseRequester),
		   strings.TrimSpace(caseCloserActor)) {
		  return fmt.Errorf("%w: actor=%s is the case closer, cannot also request release for hold=%s",
			  ErrMutex, releaseRequester, holdID)
	   }
	   return nil
  }
