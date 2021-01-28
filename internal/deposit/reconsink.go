// 功能：把入金差追加到对账工单。先开一张钱包账差异单，再写入 deposit_diff 字段。
// 不改签收、结案和栅栏。无法归因时对账用户记为 deposit-unassigned，避免空用户开单失败。

// 作者：Likeo
// 日期：2021.01.08

package deposit

import (
	"strings"
	"account-assets.kucoin.net/funding/internal/recon"
)

   func (s *Service) emitRecon(ev DepositDiffEvent) error {
	  if s == nil || s.recon == nil {
		  return nil
	  }
	   ev = NormalizeEvent(ev)
	   if ev.Kind == "" || !KnownDiffKind(ev.Kind) {
		   return nil
	  }
	   user := ev.UserID
	  if user == "" || user == "unassigned" {
		  user = "deposit-unassigned"
	   }
	  note := ev.Kind
	  if ev.Reason != "" {
		  note = ev.Kind + " " + ev.Reason
	  }
	   if len(note) > 240 {
		  note = note[:240]
	  }
	  t, err := s.recon.Open("deposit-patrol", user, "wallet", ev.Asset, note)
	   if err != nil {
		   return err
	  }
	  if t == nil {
		  return ErrTicketMiss
	   }
	   diff := recon.DepositDiff{
		  Event: ev.Event, Kind: ev.Kind, Chain: ev.Chain, TxHash: ev.TxHash,
		   OutputIndex: ev.OutputIndex, Asset: ev.Asset, UserID: ev.UserID,
		  ObservedAmount: ev.ObservedAmount, CreditedAmount: ev.CreditedAmount,
		  ObserveState: ev.ObserveState,
		   CreditState: ev.CreditState,
		   WindowStart: ev.WindowStart, WindowEnd: ev.WindowEnd,
		   SecondCredit: ev.SecondCredit, Blocked: ev.Blocked,
		   DepositTicket: ev.DepositTicket, Phase: ev.Phase,
		  ReverseEntry: ev.ReverseEntry, AvailBefore: ev.AvailBefore,
		   AvailAfter: ev.AvailAfter, Reason: ev.Reason,
	  }
	   got, err := s.recon.AppendDepositDiff(t.ID, "deposit-patrol", diff)
	   if err != nil || got == nil {
		  s.lastRecon = t.ID
		  if err != nil {
			   return err
		  }
		  return ErrTicketMiss
	   }
	   s.lastRecon = got.ID
	   return nil
  }

// ReconTicketID 最近一次追加的对账工单号。
  func (s *Service) ReconTicketID() string {
	  if s == nil {
		  return ""
	   }
	   return strings.TrimSpace(s.lastRecon)
  }
