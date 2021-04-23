package destentrygate

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// DeriveEntryID 派生稳定条目标识。不等于 ListGen / destlife Inflight gen。
  func DeriveEntryID(userID, chain, addr string) string {
	   userID = strings.TrimSpace(userID)
	  chain = strings.ToUpper(strings.TrimSpace(chain))
	   addr = strings.TrimSpace(addr)
	  sum := sha256.Sum256([]byte(userID + "|" + chain + "|" + addr))
	  return hex.EncodeToString(sum[:])
  }

// SameTuple 比较租约钉扎与提交元组。
   func SameTuple(lease DestEntryLease, userID, chain, addr string) bool {
	   return lease.UserID == strings.TrimSpace(userID) &&
		   lease.Chain == strings.ToUpper(strings.TrimSpace(chain)) &&
		   lease.Address == strings.TrimSpace(addr)
  }

// NormalizeMode 规范化条目模式。非法值返回空。
   func NormalizeMode(raw string) EntryMode {
	   switch strings.ToLower(strings.TrimSpace(raw)) {
	  case "strict":
		  return ModeStrict
	  case "temp":
		   return ModeTemp
	  case "breakglass":
		   return ModeBreakglass
	  default:
		   return ""
	   }
   }
