// 本文件在 amldesk 侧新增结案前的检疫 hold 检查钩子，
// 以及与 amlhold 包的集成接口。不改写 r6/r7 分层 taxonomy 或战争室级联协议。
// 设计：
// - amldesk 只读观察 amlhold 提供的 HoldChecker 接口。
// - amldesk 不写入 amlhold；所有 hold 状态由 amlhold.Service 管理。
// - 结案人不得独自解冻：CloseFourEyeWithHoldCheck 内同时检查互斥。

// Author: Jett
// Date: 2021.02.03

package amldesk

import (
	"fmt"
	"strings"
)

// HoldChecker 检疫 Hold 检查接口（由 amlhold.Service 实现）。
// amldesk 通过此接口只读查询 hold 状态，不直接依赖 amlhold 包（避免循环依赖）。
type HoldChecker interface {
	// CheckCloseAllowed 若 Case 下仍有未释放的 hold，返回错误。
	CheckCloseAllowed(caseID, closerActor string) error
	// CheckSameActorReleaseThenClose 若结案人已申请过解冻，返回互斥错误。
	CheckSameActorReleaseThenClose(caseID, closerActor string) error
}

// BookWithHold 在 Book 上附加 HoldChecker，提供带检疫 hold 校验的结案流程。
type BookWithHold struct {
	*Book
	holdChecker HoldChecker
}

// NewBookWithHold 构造带 hold 检查的 Book 增强版。
  func NewBookWithHold(b *Book, hc HoldChecker) *BookWithHold {
	   if b == nil {
		   b = NewBook()
	   }
	   return &BookWithHold{Book: b, holdChecker: hc}
  }

// SetHoldChecker 注入 HoldChecker（延迟注入，避免启动时循环依赖）。

  func (bh *BookWithHold) SetHoldChecker(hc HoldChecker) {
	   if bh != nil {
		   bh.holdChecker = hc
	  }
  }

// CloseFourEyeWithHoldCheck 结案前校验：
// 1. 口径四眼校验（继承 Book.CloseFourEye）。
// 2. Case 下所有检疫 hold 必须已 released（否则 ErrHoldBlocked）。
// 3. 结案人不得与已申请解冻的 actor 相同（Release↔Close Mutex）。
// SLA 超时不得自动将 Case 置为 Resolved：本函数只能由人工操作触发。
   func (bh *BookWithHold) CloseFourEyeWithHoldCheck(id, actor, decision string, epoch int64, token, note string) (Record, error) {
	   if bh == nil || bh.Book == nil {
		   return Record{}, ErrUnavailable
	  }
	   act, err := actorOK(actor)
	   if err != nil {
		   return Record{}, ErrActor
	  }
	   // 检疫 hold 校验（若未注入 holdChecker 则跳过，兼容未部署 amlhold 的环境）
	   if bh.holdChecker != nil {
		  if err := bh.holdChecker.CheckCloseAllowed(id, act); err != nil {
			  return Record{}, fmt.Errorf("%w: %v", ErrHoldBlocked, err)
		  }
		   if err := bh.holdChecker.CheckSameActorReleaseThenClose(id, act); err != nil {
			   return Record{}, fmt.Errorf("%w: %v", ErrMutexViolation, err)
		  }
	  }
	  return bh.Book.CloseFourEye(id, act, decision, epoch, token, note)
  }

// StatusResolvedCheck 专用于 StatusResolved 类操作的检查（非四眼结案，仅状态迁移）。
// 若 Case 下仍有未释放的检疫 hold，返回 ErrHoldBlocked。
  func (bh *BookWithHold) StatusResolvedCheck(caseID, actor string) error {
	   if bh == nil {
		   return ErrUnavailable
	   }
	  if bh.holdChecker == nil {
		   return nil
	   }
	   return bh.holdChecker.CheckCloseAllowed(caseID, strings.TrimSpace(actor))
   }

// QuarantineHoldSummary Case 关联的检疫 hold 摘要（只读，供 desk 展示）。
type QuarantineHoldSummary struct {
	CaseID    string `json:"case_id"`
	OpenCount int    `json:"open_hold_count"`
	HasBlocking bool   `json:"has_blocking_hold"`
	BlockReason string `json:"block_reason,omitempty"`
}

// HoldSummaryProvider 检疫 hold 摘要提供者接口。

type HoldSummaryProvider interface {
	OpenHoldsCount(caseID string) int
}

// ServiceWithHold 在 Service 上附加 BookWithHold，提供统一入口。

type ServiceWithHold struct {
	*Service
	bookWithHold *BookWithHold
}

// NewServiceWithHold 构造带 hold 检查的 Service。

   func NewServiceWithHold(hc HoldChecker) *ServiceWithHold {
	  b := NewBook()
	  bwh := NewBookWithHold(b, hc)
	  svc := &Service{book: bwh.Book, watch: NewWatchlist()}
	  return &ServiceWithHold{Service: svc,
		  bookWithHold: bwh}
   }

// Close 覆写 Service.Close：走带 hold 校验的结案流程。
  func (s *ServiceWithHold) Close(id, actor, decision string, epoch int64, token, note string) (Record, error) {
	  if s == nil || s.bookWithHold == nil {
		  return Record{}, ErrUnavailable
	   }
	  return s.bookWithHold.CloseFourEyeWithHoldCheck(id, actor, decision, epoch, token, note)
  }
// temporary guard removed after the production check
