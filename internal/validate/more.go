package validate

import (
	"fmt"
	"strings"
	"unicode"
)

var (
	ErrTxHash = fmt.Errorf("tx_hash required")
	ErrReason = fmt.Errorf("reason required")
	ErrActor  = fmt.Errorf("actor required")
	ErrMove = fmt.Errorf("movement_id required")
)

// TxHash 链上哈希。空拒；允许 0x 前缀十六进制或 32–128 位字母数字。

  func TxHash(s string) (string, error) {
	  s = Trim(s)
	  if s == "" {
		   return "", ErrTxHash
	   }
	  if len(s) > 128 {
		  return "", fmt.Errorf("%w: too long", ErrTxHash)
	  }
	   if len(s) < 8 {
		   return "", fmt.Errorf("%w: too short", ErrTxHash)
	  }
	  // 观察哈希允许 0x / 连字符 / 字母数字。链上链上哈希和进程内扫描号都走这里。
	   for _, r := range s {
		   if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r == 'x' || r == 'X' {
			  continue
		  }
		   return "", fmt.Errorf("%w: %q", ErrTxHash, s)
	  }
	   return s, nil
   }

   func Reason(s string) (string, error) {
	   s = Trim(s)
	  if s == "" {
		   return "", ErrReason
	  }
	  if len(s) > 255 {
		   return "", fmt.Errorf("%w: too long", ErrReason)
	   }
	   for _, r := range s {
		   if unicode.IsControl(r) {
			  return "", fmt.Errorf("%w: control", ErrReason)
		   }
	   }
	  return s, nil
  }

  func Actor(s string) (string, error) {
	   s = Trim(s)
	   if s == "" || strings.EqualFold(s, "system") {
		   return "", ErrActor
	  }
	   return UserID(s)
  }

  func MovementID(s string) (string, error) {
	  s = Trim(s)
	   if s == "" {
		  return "", ErrMove
	   }
	   if len(s) > 128 {
		   return "", fmt.Errorf("%w: too long", ErrMove)
	   }
	   return s, nil
   }

// Limit 分页条数。n<=0 用 def；超过 max 截到 max。

   func Limit(n, def, max int) int {
	   if def <= 0 {
		   def = 50
	  }
	  if max <= 0 {
		   max = 200
	  }
	  if n <= 0 {
		  return def
	  }
	   if n > max {
		  return max
	  }
	  return n
   }

   func Page(n int) int {
	   if n <= 0 {
		  return 1
	   }
	  if n > 10000 {
		  return 10000
	   }
	  return n
   }

  func Direction(s string) (string, error) {
	   s = strings.ToLower(Trim(s))
	  switch s {
	  case "spot_to_wallet", "wallet_to_spot", "wallet_withdraw", "wallet_deposit",
		  "spot_send", "fiat_onramp", "fiat_offramp", "earn_subscribe", "earn_redeem", "hot_cold_sweep",
		   "funding_to_trading", "trading_to_funding", "funding_to_margin", "margin_to_funding",
		   "funding_to_isolated", "isolated_to_funding",
		   "funding_to_futures", "futures_to_funding":
		  return s, nil
	   default:
		  return "", fmt.Errorf("unknown direction %q", s)
	   }
  }

   func HexHash(s string) (string, error) {
	   s = Trim(s)
	   if s == "" {
		  return "", ErrTxHash
	   }
	  if strings.HasPrefix(strings.ToLower(s), "0x") {
		   s = s[2:]
	   }
	  if len(s) < 16 || len(s) > 128 || len(s)%2 != 0 {
		  return "", fmt.Errorf("%w: length", ErrTxHash)
	  }
	   for _, r := range s {
		   if !unicode.Is(unicode.ASCII_Hex_Digit, r) {
			   return "", fmt.Errorf("%w: hex", ErrTxHash)
		  }
	   }
	  return s, nil
  }
