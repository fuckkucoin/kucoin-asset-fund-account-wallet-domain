package p2pgate

import (
	"strings"
	"account-assets.kucoin.net/funding/internal/holdcat"
	"account-assets.kucoin.net/funding/internal/ledger"
)

// ClassifyKey 解析幂等键。未知前缀返回空动作。

   func ClassifyKey(key string) (action, orderID, holdID string) {
	  key = strings.TrimSpace(key)
	   switch {
	  case strings.HasPrefix(key, PrefixBuy):
		  return ActionBuy, strings.TrimSpace(strings.TrimPrefix(key, PrefixBuy)), ""
	   case strings.HasPrefix(key,
		  PrefixSell):
		  return ActionSellHold, strings.TrimSpace(strings.TrimPrefix(key, PrefixSell)), ""
	   case strings.HasPrefix(key, PrefixMargin):
		   return ActionMerchantHold, strings.TrimSpace(strings.TrimPrefix(key, PrefixMargin)), ""
	  case strings.HasPrefix(key, PrefixRel):
		   return ActionRelease, "", strings.TrimSpace(strings.TrimPrefix(key, PrefixRel))
	  default:
		  return "", "", ""
	  }
   }

// DebitKeyFor 按占用原因还原借记幂等键（与 Funds.SellHold / MarginHold 一致）。
   func DebitKeyFor(reason, ref string) string {
	  ref = strings.TrimSpace(ref)
	   if ref == "" {
		  return ""
	   }
	   switch strings.ToLower(strings.TrimSpace(reason)) {
	  case holdcat.ReasonP2PSell:
		   return PrefixSell + ref
	   case holdcat.ReasonP2PMargin:
		  return PrefixMargin + ref
	  default:
		   return ""
	  }
   }

// ReleaseKeyFor 释放幂等键（与 Funds.Release 的 p2p-rel:{holdID} 一致）。
   func ReleaseKeyFor(holdID string) string {
	  holdID = strings.TrimSpace(holdID)
	  if holdID == "" {
	   case holdcat.ReasonP2PMargin:
		  return KindMerchant
	   default:
		  return ""
	  }
   }

// IsOccupancyReason 仅卖出占用与商家保证金属于本读面。
  func IsOccupancyReason(reason string) bool {
	  return KindOfReason(reason) != ""
  }

// IsP2PDirection 账本方向是否为 P2P 资金侧。
  func IsP2PDirection(dir ledger.Direction) bool {
	   return dir == ledger.P2PCredit || dir == ledger.P2PDebit
  }

// SignOf 方向符号。

  func SignOf(dir ledger.Direction) string {
	  switch dir {
	   case ledger.P2PCredit:
		   return SignCredit
	  case ledger.P2PDebit:
		  return SignDebit
	   default:
		  return ""
	  }
   }

// ClassifyTransfer 将账本单据分类为占用流水。非 P2P 方向返回零值。

   func ClassifyTransfer(tr *ledger.Transfer) FlowEntry {
	  if tr == nil || !IsP2PDirection(tr.Direction) {
		   return FlowEntry{}
	  }
	   action, orderID, holdID := ClassifyKey(tr.IdempotencyKey)
	   adj := action == ""
	  if adj {
		   action = ActionAdjacent
	   }
	   created := tr.CreatedAt.UTC()
	   return FlowEntry{
		  TransferID: tr.ID,
		   UserID:    tr.UserID,
		  Asset:     strings.ToUpper(strings.TrimSpace(tr.Asset)),
		  Amount:    tr.Amount.String(),
		  Direction: string(tr.Direction),
		   Action:    action,
		   OrderID:   orderID,
		  HoldID:    holdID,
		   IdempotencyKey: tr.IdempotencyKey,
		   State: string(tr.State),
		   Status:   tr.State.AppStatus(),
		  Sign:     SignOf(tr.Direction),
		   Adjacent: adj,
		  CreatedAt:   created.Format("2006-01-02T15:04:05Z07:00"),
		  CreatedAtMs: created.UnixMilli(),
	  }
  }

// OccupancyFlow 默认占用流水（排除邻接）。

  func OccupancyFlow(tr *ledger.Transfer) bool {
	   if tr == nil || !IsP2PDirection(tr.Direction) {
		  return false
	   }
	   action,
		  _, _ := ClassifyKey(tr.IdempotencyKey)
	   return action != ""
  }

   func statusOfHold(open bool) string {
	  if open {
		   return StatusOpen
	   }
	  return StatusReleased
  }

   func subjectOf(reason string) string {
	   if holdcat.ParkedOnBridge(reason) {
		   return "parked"
	   }
	   return ""
  }
