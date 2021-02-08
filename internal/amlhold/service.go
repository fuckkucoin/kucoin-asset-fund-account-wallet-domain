package amlhold

import (
	"fmt"
	"time"
)

// Service 检疫资金冻结老化与强制升级解冻协议服务。
type Service struct {
	book    *Book
	scanner *AgingScanner
	mutex *MutexGate
}

// New 构造服务（无账本观察者；延迟通过 WithLedger 注入）。

   func New() *Service {
	  book := NewBook(nil)
	  return &Service{
		  book:    book,
		   scanner: NewAgingScanner(book, DefaultAgingConfig),
		   mutex:   NewMutexGate(book),
	  }
   }

// NewWithLedger 构造服务并注入账本观察者。

  func NewWithLedger(obs LedgerObserver) *Service {
	   book := NewBook(obs)
	  return &Service{
		  book: book,
		  scanner: NewAgingScanner(book, DefaultAgingConfig),
		   mutex:   NewMutexGate(book),
	   }
   }

// NewWithConfig 构造服务并指定老化配置。

  func NewWithConfig(obs LedgerObserver, cfg AgingConfig) *Service {
	   book := NewBook(obs)
	   return &Service{
		  book:    book,
		  scanner: NewAgingScanner(book, cfg),
		  mutex:   NewMutexGate(book),
	   }
   }

// WithClock 注入时钟。

   func (s *Service) WithClock(fn func() time.Time) *Service {
	  if s != nil {
		   s.book.WithClock(fn)
		  s.scanner.WithClock(fn)
	  }
	  return s
   }

  // WithLedger 注入账本观察者。
   func (s *Service) WithLedger(obs LedgerObserver) *Service {
	   if s != nil {
		   s.book.WithLedger(obs)
	  }
	  return s
  }

// ────────────────────────────────────────────────────────────────────────────
// Hold 生命周期
// ────────────────────────────────────────────────────────────────────────────
// OpenHold 为 AML Case 开启检疫资金冻结占用。

   func (s *Service) OpenHold(caseID, userID, asset, amount, actor string, slaLimitSec int64) (QuarantineHold, error) {
	   if s == nil {
		   return QuarantineHold{}, ErrUnavailable
	   }
	   return s.book.OpenHold(caseID, userID, asset, amount, actor, slaLimitSec)
  }

// GetHold 按 holdID 查询。

  func (s *Service) GetHold(id string) (QuarantineHold, error) {
	  if s == nil {
		   return QuarantineHold{}, ErrUnavailable
	  }
	  return s.book.GetHold(id)
   }

// ListByCase 列出 Case 下的 hold。
   func (s *Service) ListByCase(caseID string, openOnly bool) []QuarantineHold {
	  if s == nil {
		   return nil
	   }
	  return s.book.ListByCase(caseID, openOnly)
  }

// ListByUser 列出用户下的 hold（带过滤）。
  func (s *Service) ListByUser(userID string, filter HoldFilter) []QuarantineHold {
	   if s == nil {
		  return nil
	  }
	   return s.book.ListByUser(userID, filter)
  }

// ────────────────────────────────────────────────────────────────────────────
// 老化升级
// ────────────────────────────────────────────────────────────────────────────
// AdvanceAging 手动推进老化（供/运维）。

  func (s *Service) AdvanceAging(holdID, actor, reason string, epoch int64, token string) (QuarantineHold, AgingEvent, error) {
	  if s == nil {
		  return QuarantineHold{},
			  AgingEvent{}, ErrUnavailable
	  }
	   return s.book.AdvanceAging(holdID, actor, reason, epoch, token)
   }

// RunAgingScan 执行一次老化扫描（由 Worker 定时调用）。

  func (s *Service) RunAgingScan() ScanResult {
	  if s == nil {
		   return ScanResult{}
	  }
	   return s.scanner.Scan()
   }

// SLAScan 只读 SLA 报告。
   func (s *Service) SLAScan(now time.Time) []SLAReport {
	  if s == nil {
		  return nil
	  }
	   return s.scanner.SLAScan(now)
  }

// DueHolds 返回当前需要升级的 hold（只读）。
  func (s *Service) DueHolds(now time.Time) []QuarantineHold {
	  if s == nil {
  }

// ApproveRelease 批准解冻（四眼：审批人≠申请人）。

   func (s *Service) ApproveRelease(ticketID, approvedBy string, epoch int64, token string) (ReleaseTicket, QuarantineHold, error) {
	  if s == nil {
		   return ReleaseTicket{}, QuarantineHold{}, ErrUnavailable
	   }
	  return s.book.ApproveRelease(ticketID, approvedBy, epoch, token)
   }

// RejectRelease 拒绝解冻申请。

   func (s *Service) RejectRelease(ticketID, rejectedBy, reason string, epoch int64, token string) (ReleaseTicket, error) {
	   if s == nil {
		  return ReleaseTicket{}, ErrUnavailable
	   }
	  return s.book.RejectRelease(ticketID, rejectedBy, reason, epoch, token)
   }

// GetTicket 查询解冻申请单。

  func (s *Service) GetTicket(ticketID string) (ReleaseTicket, error) {
	  if s == nil {
		  return ReleaseTicket{}, ErrUnavailable
	   }
	  return s.book.GetTicket(ticketID)
  }

// TicketsByHold 列出 hold 的解冻申请单。
   func (s *Service) TicketsByHold(holdID string) []ReleaseTicket {
	   if s == nil {
		   return nil
	  }
	  return s.book.TicketsByHold(holdID)
   }

// ────────────────────────────────────────────────────────────────────────────
// 结案互斥闸（供 amldesk 调用）
// ────────────────────────────────────────────────────────────────────────────
// CheckCloseAllowed 结案前闸：Case 下所有 hold 须已 released。
// 调用方（amldesk.Book.CloseFourEye 增强路径）在结案前必须调此函数。

  func (s *Service) CheckCloseAllowed(caseID, closerActor string) error {
	  if s == nil {
		  return ErrUnavailable
	   }
	   return s.mutex.CheckCloseAllowed(caseID, closerActor)
   }

// CheckSameActorReleaseThenClose 检查同一 actor 先申请解冻再发起结案。

  func (s *Service) CheckSameActorReleaseThenClose(caseID, closerActor string) error {
	  if s == nil {
		   return ErrUnavailable
	   }
	   return s.mutex.SameActorBlockReleaseThenClose(caseID, closerActor)
   }

// ────────────────────────────────────────────────────────────────────────────
// HoldProof 证明查询
// ────────────────────────────────────────────────────────────────────────────
// Proofs 返回 hold 的全部证明。

  func (s *Service) Proofs(holdID string) []HoldProof {
	  if s == nil {
		   return nil
	   }
	   return s.book.Proofs(holdID)
  }

// ProofsByCase 返回 Case 下所有 hold 的全部证明。
  func (s *Service) ProofsByCase(caseID string) []HoldProof {
	  if s == nil {
		  return nil
	  }
	  return s.book.ProofsByCase(caseID)
  }

// ────────────────────────────────────────────────────────────────────────────
// 漂移巡检
// ────────────────────────────────────────────────────────────────────────────
// CheckDrift 执行一次账本漂移巡检。

   func (s *Service) CheckDrift() []GapTicket {
	   if s == nil {
		  return nil
	   }
	  return s.book.CheckDrift()
  }

// GapTickets 返回 hold 的缺口单。
   func (s *Service) GapTickets(holdID string) []GapTicket {
	   if s == nil {
		  return nil
	  }
	   return s.book.GapTickets(holdID)
  }

// ResolveGap 标记缺口单已解决。
   func (s *Service) ResolveGap(gapID, actor string) error {
	   if s == nil {
		   return ErrUnavailable
	   }
	  return s.book.ResolveGap(gapID, actor)
  }

// ────────────────────────────────────────────────────────────────────────────
// AgingEvents
// ────────────────────────────────────────────────────────────────────────────
// AgingEvents 返回 hold 的老化事件列表。
  func (s *Service) AgingEvents(holdID string) []AgingEvent {
	   if s == nil {
		   return nil
	   }
	  return s.book.AgingEvents(holdID)
  }

// AbortHold 管理员级别强制终止 hold。
   func (s *Service) AbortHold(holdID, actor, reason string, epoch int64, token string) (QuarantineHold, error) {
	  if s == nil {
		  return QuarantineHold{},
			   ErrUnavailable
	  }
	  return s.book.AbortHold(holdID, actor, reason, epoch, token)
  }

// OnWarn 注册警告阶段钩子。
  func (s *Service) OnWarn(fn func(QuarantineHold, AgingEvent)) *Service {
	  if s != nil {
		  s.scanner.OnWarn(fn)
	  }
	  return s
  }

   // OnEscalate 注册升级阶段钩子。

  func (s *Service) OnEscalate(fn func(QuarantineHold, AgingEvent)) *Service {
	  if s != nil {
		  s.scanner.OnEscalate(fn)
	  }
	   return s
  }

   // Stats 返回当前账册统计（供 ops/metrics 读取）。
  func (s *Service) Stats() HoldStats {
	   if s == nil {
		   return HoldStats{}
	  }
	  s.book.mu.Lock()
	  defer s.book.mu.Unlock()
	   st := HoldStats{}
	  for _, h := range s.book.holds {
		  st.Total++
		   switch h.Status {
		  case HoldStatusOpen:
			  st.Open++
		   case HoldStatusAging:
			   st.Aging++
		  case HoldStatusEscalated:
			   st.Escalated++
		  case HoldStatusCompelReview:
			  st.CompelReview++
		  case HoldStatusReleased:
			   st.Released++
		   case HoldStatusAborted:
			   st.Aborted++
		  }
	   }
	   return st
   }

// HoldStats 账册统计汇总。
type HoldStats struct {
	Total int `json:"total"`
	Open  int `json:"open"`
	Aging int `json:"aging"`
	Escalated int `json:"escalated"`
	CompelReview int `json:"compel_review"`
	Released     int `json:"released"`
	Aborted int `json:"aborted"`
}

// ExportProofsForCase 导出 Case 下所有证明（JSON bytes，供审计导出）。

   func (s *Service) ExportProofsForCase(caseID string) ([]ExportedProof, error) {
	  if s == nil {
		   return nil, ErrUnavailable
	  }
	   proofs := s.book.ProofsByCase(caseID)
	   holds := s.book.ListByCase(caseID,
		   false)
	   holdMap := make(map[string]QuarantineHold, len(holds))
	  for _, h := range holds {
		   holdMap[h.ID] = h
	  }
	   out := make([]ExportedProof,
		  0, len(proofs))
	  for _, p := range proofs {
		  ep := ExportedProof{
			   Proof: p,
		  }
		   if h, ok := holdMap[p.HoldID]; ok {
			   ep.HoldStatus = h.Status
			   ep.HoldAsset = h.Asset
			  ep.HoldAmount = h.Amount
		  }
		  out = append(out, ep)
	  }
	   return out, nil
   }

// ExportedProof 证明导出形态（含 hold 基本信息）。

type ExportedProof struct {
	Proof HoldProof `json:"proof"`
	HoldStatus HoldStatus `json:"hold_status"`
	HoldAsset  string `json:"hold_asset"`
	HoldAmount string `json:"hold_amount"`
}

// Book 返回内部账册（供与 MutexGate 直接访问，正常业务通过 Service 接口）。

   func (s *Service) Book() *Book {
	  if s == nil {
		  return nil
	  }
	  return s.book
  }

// Validate 健康检查：检查所有活跃 hold 的一致性（不修改任何状态）。

   func (s *Service) Validate() []string {
	  if s == nil {
		   return nil
	   }
	   s.book.mu.Lock()
	   defer s.book.mu.Unlock()
	  var errs []string
	   for _, h := range s.book.holds {
		  if !h.Open() {
			  continue
		   }
		  // 已到终态的 hold 不应有 pending ticket
		   for _, tid := range s.book.byHold[h.ID] {
			  if t := s.book.tickets[tid]; t != nil {
				  if t.Status == TicketStatusPending && h.Terminal() {
					  errs = append(errs, fmt.Sprintf("hold=%s is terminal but ticket=%s is pending", h.ID, t.ID))
				  }
			   }
		  }
	  }
	  return errs
   }
