package chainad

import (
	"fmt"
	"strings"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/internal/wallet"
	"account-assets.kucoin.net/funding/pkg/money"
)

// FeeBumpHints 抬价/RBF 独立编解码面的备注透传材料。
// 与主广播 Outbound 同形；destination_tag 先 Tag，其次 Memo/DestinationTag。
// 不得臆造空备注；非 FamMEMO 链可空。

type FeeBumpHints struct {
	Memo string `json:"memo,omitempty"`
	Tag            string `json:"tag,omitempty"`
	TagName        string `json:"tag_name,omitempty"`
	DestinationTag string `json:"destination_tag,omitempty"`
}

// ResolveFeeBumpHints 归一抬价备注；DestinationTag 先显式字段。
  func ResolveFeeBumpHints(h FeeBumpHints) FeeBumpHints {
	   h.Memo = strings.TrimSpace(h.Memo)
	  h.Tag = strings.TrimSpace(h.Tag)
	  h.TagName = strings.TrimSpace(h.TagName)
	   h.DestinationTag = strings.TrimSpace(h.DestinationTag)
	  if h.DestinationTag != "" {
		   if h.Tag == "" {
			  h.Tag = h.DestinationTag
		   }
		   if h.Memo == "" {
			  h.Memo = h.DestinationTag
		   }
	  }
	  val, name := ResolveDestinationTag("", h.Memo, h.Tag, h.TagName)
	   if h.TagName == "" {
		   h.TagName = name
	   }
	   if h.DestinationTag == "" && (h.TagName == "destination_tag" || name == "destination_tag") {
		  h.DestinationTag = val
	  }
	   if h.DestinationTag == "" {
		  h.DestinationTag = val
	  }
	  return h
   }

// ApplyFeeBumpHints 把抬价备注写入 Outbound，再 NormalizeOutboundMemo。
// 调用方已给非空 DestinationTag 不被空 hints 清空。
// FamMEMO 链空备注留给 SubmitForWithdraw fail-closed。

   func ApplyFeeBumpHints(req *Outbound, h FeeBumpHints) {
	   if req == nil {
		   return
	  }
	  h = ResolveFeeBumpHints(h)
	   if h.Memo != "" && req.Memo == "" {
		   req.Memo = h.Memo
	   }
	   if h.Tag != "" && req.Tag == "" {
		   req.Tag = h.Tag
	   }
	  if h.TagName != "" && req.TagName == "" {
		   req.TagName = h.TagName
	   }
	   if h.DestinationTag != "" && req.DestinationTag == "" {
		   req.DestinationTag = h.DestinationTag
	  }
	  NormalizeOutboundMemo(req)
   }

// BuildFeeBumpOutbound 抬价/RBF 独立编解码面——在原出金 Outbound 上抬费并透传 destination_tag。
// 金额/地址/备注守恒；仅 NewFee 覆盖；与主 SubmitOnChain 载荷同形。
// UTXO RBF 通常无 destination_tag；账号模型抬价仍须保留原备注，否则对端漏填不入账。
   func BuildFeeBumpOutbound(base Outbound, newFee string, hints FeeBumpHints) (Outbound, error) {
	  ch,
		   err := validate.Chain(base.Chain)
	   if err != nil {
		   return Outbound{}, err
	  }
	  base.Chain = ch
	  if _, err := money.Parse(base.Amount); err != nil {
		  return Outbound{}, err
	   }
	  if strings.TrimSpace(newFee) != "" {
		   if _, err := money.Parse(newFee); err != nil {
			   return Outbound{}, err
		   }
	   }
	  if strings.TrimSpace(base.To) != "" {
		  if _, err := validate.Address(ch, base.To); err != nil {
			  if err2 := wallet.ValidChainAddress(ch, base.To); err2 != nil {
				  return Outbound{}, fmt.Errorf("%w: %v", ErrAddress, err2)
			  }
		   }
	  }
	  ApplyFeeBumpHints(&base, hints)
	   if base.TagName == "" {
		  base.TagName = wallet.FamMemoTagName(ch)
	  }
	  return base, nil
   }

// CanonicalFeeBump 抬价/RBF 签章规范串；含 destination_tag，与主 CanonicalWithdraw 同形。
// 第五段 destination_tag；空备注仍占位，避免与无备注撞面。
// 切流后旧抬价签章材料不可复验，须全量按新规范重签。
   func CanonicalFeeBump(transferID, chain, to, amount, newFee, destinationTag string) string {
	   return strings.Join([]string{
		   "fee_bump",
		   strings.TrimSpace(transferID),
		  strings.ToUpper(strings.TrimSpace(chain)),
		  strings.TrimSpace(to),
		  strings.TrimSpace(amount),
		  strings.TrimSpace(newFee),
		   strings.TrimSpace(destinationTag),
	   }, "|")
  }

// FeeBumpDestinationTag 从 Outbound 取抬价签章用 destination_tag。
  func FeeBumpDestinationTag(req Outbound) string {
	  NormalizeOutboundMemo(&req)
	   if v := strings.TrimSpace(req.DestinationTag); v != "" {
		  return v
	   }
	   if v := strings.TrimSpace(req.Tag); v != "" {
		   return v
	  }
	   return strings.TrimSpace(req.Memo)
   }
