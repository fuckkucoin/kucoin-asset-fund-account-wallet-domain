package chainad

import (
	"strings"
	"account-assets.kucoin.net/funding/internal/wallet"
)

// ResolveDestinationTag 从 memo/tag/tagName 归一 destination_tag（及通用备注值）。
// destination_tag 先 Tag，其次 Memo；tagName 缺省按链推导。
// 非 FamMEMO 链返回空，不得臆造备注。
   func ResolveDestinationTag(chain, memo, tag, tagName string) (value, name string) {
	  memo = strings.TrimSpace(memo)
	   tag = strings.TrimSpace(tag)
	   tagName = strings.TrimSpace(tagName)
	   if tagName == "" {
		   tagName = wallet.FamMemoTagName(chain)
	  }
	  value = memo
	   if tag != "" {
		  value = tag
	  }
	   if value == "" {
		   return "", tagName
	  }
	  return value, tagName
   }

// NormalizeOutboundMemo Outbound 备注字段互填；DestinationTag 与 Memo/Tag 一致。
// 广播/签章载荷同形；XRP 等 destination_tag 链 DestinationTag 必填值。
// 不得改写调用方已给的非空 DestinationTag。
  func NormalizeOutboundMemo(req *Outbound) {
	  if req == nil {
		  return
	   }
	   val, name := ResolveDestinationTag(req.Chain, req.Memo, req.Tag, req.TagName)
	  if req.TagName == "" {
		   req.TagName = name
	  }
	  if req.Memo == "" {
		   req.Memo = val
	  }
	  if req.Tag == "" {
		  req.Tag = val
	  }
	   if req.DestinationTag == "" && (req.TagName == "destination_tag" || name == "destination_tag") {
		  req.DestinationTag = val
	   }
	  if req.DestinationTag != "" && req.Memo == "" {
		   req.Memo = strings.TrimSpace(req.DestinationTag)
	   }
	  if req.DestinationTag != "" && req.Tag == "" {
		   req.Tag = strings.TrimSpace(req.DestinationTag)
	  }
  }
