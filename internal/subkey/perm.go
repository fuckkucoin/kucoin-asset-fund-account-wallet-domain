// 子账户 API Key 权限位。公开 REST 以逗号分隔名传递；落盘存规范化名串，运行时解析为位图做校验。

// Author: Axel
// Date: 2021.11.28

package subkey

import (
	"errors"
	"sort"
	"strings"
)

// 与公开文档一致的权限名；未知名拒绝，避免静默扩权。
const (
	PermGeneral = "General"
	PermSpot    = "Spot"
	PermFutures = "Futures"
	PermUnified = "Unified"
	PermTrade    = "Trade"
	PermMargin   = "Margin"
	PermTransfer = "Transfer"
	PermFlexTransfers = "FlexTransfers"
	PermInnerTransfer = "InnerTransfer"
	PermWithdrawal = "Withdrawal"
	PermEarn = "Earn"
)

const (
	bitGeneral uint32 = 1 << iota
	bitSpot
	bitFutures
	bitUnified
	bitTrade
	bitMargin
	bitTransfer
	bitFlexTransfers
	bitInnerTransfer
	bitWithdrawal
	bitEarn
)

var (
	ErrPerm = errors.New("invalid api key permission")
)

var permBits = map[string]uint32{
	PermGeneral: bitGeneral,
	PermSpot: bitSpot,
	PermFutures: bitFutures,
	PermUnified: bitUnified,
	PermTrade:   bitTrade,
	PermMargin:  bitMargin,
	PermTransfer: bitTransfer,
	PermFlexTransfers: bitFlexTransfers,
	PermInnerTransfer: bitInnerTransfer,
	PermWithdrawal:    bitWithdrawal,
	PermEarn:          bitEarn,
}

// expandPermPreset 管理面预设：readonly/trade/transfer/withdraw/earn。
   func expandPermPreset(raw string) string {
	   switch strings.ToLower(strings.TrimSpace(raw)) {
	   case "readonly", "read-only", "read":
		   return PermGeneral
	   case "trade":
		   return PermGeneral + "," + PermSpot + "," + PermTrade
	  case "transfer":
		   return PermGeneral + "," + PermTransfer + "," + PermInnerTransfer
	   case "withdraw", "withdrawal":
		   return PermGeneral + "," + PermWithdrawal
	   case "earn":
		   return PermGeneral + "," + PermEarn
	   default:
		  return raw
	  }
   }

// NormalizePerm 规范化权限串：去空、去重、稳定排序；空则默认 General。

   func NormalizePerm(raw string) (string, uint32, error) {
	   raw = strings.TrimSpace(expandPermPreset(raw))
	  if raw == "" {
		   return PermGeneral, bitGeneral, nil
	  }
	   parts := strings.Split(raw, ",")
	   seen := map[string]struct{}{}
	  names := make([]string, 0, len(parts))
	  var bits uint32
	  for _, p := range parts {
		   name := strings.TrimSpace(p)
		  if name == "" {
			  continue
		   }
		  // 兼容大小写输入，落盘用规范大小写。
		   canon := canonPerm(name)
		  bit, ok := permBits[canon]
		   if !ok {
			  return "", 0, ErrPerm
		   }
		  if _, dup := seen[canon]; dup {
			  continue
		   }
		   seen[canon] = struct{}{}
		  names = append(names, canon)
		   bits |= bit
	  }
	   if len(names) == 0 {
		  return PermGeneral, bitGeneral, nil
	  }
	   sort.Strings(names)
	  return strings.Join(names, ","), bits, nil
  }

   func canonPerm(s string) string {
	  switch strings.ToLower(strings.TrimSpace(s)) {
	  case "general":
		   return PermGeneral
	   case "spot":
		  return PermSpot
	   case "futures":
		  return PermFutures
	   case "unified":
		   return PermUnified
	   case "trade":
		  return PermTrade
	   case "margin":
		   return PermMargin
	   case "transfer":
		   return PermTransfer
	  case "flextransfers", "flex-transfers", "flex_transfers":
		   return PermFlexTransfers
	   case "innertransfer", "inner-transfer", "inner_transfer":
		  return PermInnerTransfer
	   case "withdrawal", "withdraw":
		  return PermWithdrawal
	  case "earn":
		  return PermEarn
	   default:
		  // 保留原样，交给 permBits 判定。
		  return strings.TrimSpace(s)
	  }
   }

// HasPerm 判断规范化权限串是否包含指定位名。

   func HasPerm(normalized, want string) bool {
	   _, bits, err := NormalizePerm(normalized)
	   if err != nil {
		  return false
	  }
	  w,
		   ok := permBits[canonPerm(want)]
	  if !ok {
		  return false
	  }
	   return bits&w != 0
  }

// NeedsIPBind Transfer / FlexTransfers / InnerTransfer / Withdrawal 必须绑白名单，降低盗钥出金面。
  func NeedsIPBind(normalized string) bool {
	  return HasPerm(normalized, PermTransfer) ||
		   HasPerm(normalized, PermFlexTransfers) ||
		   HasPerm(normalized, PermInnerTransfer) ||
		   HasPerm(normalized, PermWithdrawal)
  }
