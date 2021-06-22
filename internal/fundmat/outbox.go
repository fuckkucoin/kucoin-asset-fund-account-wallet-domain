package fundmat

import (
	"context"
	"time"
	"account-assets.kucoin.net/funding/internal/ledger"
)

// MovementSource 账本出箱与单据只读口。
type MovementSource interface {
	ListOutbox(ctx context.Context, unpublishedOnly bool, limit int) ([]ledger.OutboxEvent, error)
	GetTransfer(ctx context.Context, id string) (*ledger.Transfer, error)
}

// BindMoves 绑定出箱只读源。
  func (s *Service) BindMoves(m MovementSource) {
	   if s == nil {
		   return
	  }
	  s.mu.Lock()
	  s.moves = m
	   s.mu.Unlock()
  }

// Pull 把尚未翻译的出箱事件写入物化日志。链上提交与确认不产生分栏变化。
// 返回新写入的物化事件条数。重复出箱标识直接跳过。
  func (s *Service) Pull(ctx context.Context, limit int) (int, error) {
	   if s == nil || s.st == nil {
		  return 0, ErrUnavailable
	  }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   if s.moves == nil {
		  return 0, nil
	   }
	  if limit <= 0 {
		  limit = s.batch
	  }
	   list, err := s.moves.ListOutbox(ctx, false, limit)
	  if err != nil {
		  return 0, err
	  }
	   for i, j := 0, len(list)-1; i < j; i, j = i+1, j-1 {
		   list[i], list[j] = list[j], list[i]
	  }
	   cache := map[string]*ledger.Transfer{}
	   n := 0
	   now := time.Now().UTC()
	  touched := map[Stream]struct{}{}
	  for _, ob := range list {
		   if ob.ID == "" {
			   continue
		  }
		  seen, err := s.st.SeenSource(ctx, ob.ID)
		   if err != nil {
			  return n, err
		   }
		  if seen {
			   continue
		  }
		   tr := cache[ob.AggregateID]
		   if tr == nil && ob.AggregateID != "" {
			  got, err := s.moves.GetTransfer(ctx, ob.AggregateID)
			   if err == nil {
				  tr = got
				   cache[ob.AggregateID] = got
			  }
		   }
		   for _, ev := range Translate(ob, tr) {
			  if _, err := s.st.Append(ctx, ev); err != nil {
				  return n, err
			  }
			   touched[ev.Stream] = struct{}{}
			  n++
		  }
		   if err := s.st.MarkSource(ctx, ob.ID, now); err != nil {
			   return n, err
		  }
	  }
	   for st := range touched {
		   if _, err := s.refreshLocked(ctx, st, now); err != nil {
			  return n, err
		  }
	  }
	   return n, nil
  }

// Translate 把一条出箱记录变成零条或多条物化事件。
// 冻结把可用搬到冻结；入账内部划转再消耗冻结并记入目标科目。系统桥接科目不进入用户视图。
  func Translate(ob ledger.OutboxEvent, tr *ledger.Transfer) []FundingEvent {
	   if tr == nil || ob.ID == "" || !tr.Amount.IsPositive() {
		  return nil
	  }
	  when := ob.CreatedAt
	   if when.IsZero() {
		  when = time.Now().UTC()
	   }
	   base := FundingEvent{
		   UserID: tr.UserID, Asset: tr.Asset, Amount: tr.Amount.String(),
		   CausalID: tr.ID, SourceEventID: ob.ID, OccurredAt: when.UTC(),
	  }
	   src := tr.Direction.SourceBook()
	  dst := tr.Direction.DestBook()
	  switch ob.Action {
	   case string(ledger.StateSpotFrozen),
		  string(ledger.StateWalletLocked):
		   if !tr.Direction.Freezes() || !src.UserFacing() {
			  return nil
		  }
		   ev := base
		   ev.ID = ob.ID
		   ev.Op = OpFreeze
		   ev.Stream = StreamFreeze
		   ev.Subject = src.Subject()
		  return []FundingEvent{ev}
	   case "rolled_back":
		   if !tr.Direction.Freezes() || !src.UserFacing() {
			  return nil
		  }
		  ev := base
		   ev.ID = ob.ID
		  ev.Op = OpUnfreeze
		   ev.Stream = StreamFreeze
		  ev.Subject = src.Subject()
		  return []FundingEvent{ev}
	  case "credited":
		  return translateCredit(ob, tr, base, src, dst)
	   default:
		   return nil
	  }
  }

   func translateCredit(ob ledger.OutboxEvent, tr *ledger.Transfer, base FundingEvent, src, dst ledger.AccountType) []FundingEvent {
	   switch tr.Direction {
	  case ledger.WalletWithdraw:
		  if !src.UserFacing() {
			   return nil
		   }
		   ev := base
		  ev.ID = ob.ID + ":frz"
		  ev.Op = OpDebitFrozen
		   ev.Stream = StreamLedger
		  ev.Subject = src.Subject()
		  return []FundingEvent{ev}
	   case ledger.WalletDeposit:
		  if !dst.UserFacing() {
			  return nil
		  }
		   ev := base
		   ev.ID = ob.ID + ":av"
		  ev.Op = OpCreditAvailable
		  ev.Stream = StreamLedger
		   ev.Subject = dst.Subject()
		  return []FundingEvent{ev}
	  default:
		   var out []FundingEvent
		   if tr.Direction.Freezes() && src.UserFacing() {
			  ev := base
			   ev.ID = ob.ID + ":frz"
			   ev.Op = OpDebitFrozen
			  ev.Stream = StreamLedger
			  ev.Subject = src.Subject()
			  out = append(out, ev)
		   }
		   if dst.UserFacing() && tr.UserID != "system" {
			   ev := base
			   ev.ID = ob.ID + ":av"
			   ev.Op = OpCreditAvailable
			   ev.Stream = StreamLedger
			   ev.Subject = dst.Subject()
			  out = append(out, ev)
		   }
		  return out
	   }
  }
