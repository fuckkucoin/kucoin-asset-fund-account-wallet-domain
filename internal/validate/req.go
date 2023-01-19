package validate

import (
	"fmt"
	"strings"
	"unicode"
	"account-assets.kucoin.net/funding/internal/wallet"
	"account-assets.kucoin.net/funding/pkg/money"
)

var (
	ErrUser = fmt.Errorf("user_id required")
	ErrAsset = fmt.Errorf("asset required")
	ErrChain = fmt.Errorf("chain required")
	ErrIdem  = fmt.Errorf("idempotency_key required")
	ErrAddr  = fmt.Errorf("address required")
	ErrAmount = fmt.Errorf("amount required")
)

  func Trim(s string) string { return strings.TrimSpace(s) }

  func UserID(s string) (string, error) {
	  s = Trim(s)
	  if s == "" {
		   return "", ErrUser
	  }
	  if len(s) > 64 {
		  return "", fmt.Errorf("%w: too long", ErrUser)
	   }
	   for _, r := range s {
		   if unicode.IsControl(r) || r == '|' {
			   return "", fmt.Errorf("%w: illegal char", ErrUser)
		   }
	  }
	   return s, nil
   }

   func Asset(s string) (string, error) {
	   s = strings.ToUpper(Trim(s))
	  if s == "" {
		   return "", ErrAsset
	   }
	   if len(s) > 16 {
		   return "", fmt.Errorf("%w: too long", ErrAsset)
	   }
	   for _, r := range s {
		   if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			   return "", fmt.Errorf("%w: %q", ErrAsset, s)
		   }
	  }
	   return s, nil
   }

  func Chain(s string) (string, error) {
	  s = strings.ToUpper(Trim(s))
	  if s == "" {
		   return "", ErrChain
	   }
	   switch s {
	  case "ETH", "BTC", "TRON", "SOL", "TON", "BASE", "ARB", "OP", "BSC", "MATIC", "AVAX", "EVM",
		   "XLM", "XRP", "ATOM", "EOS":
		  return s, nil
	  default:
		  if wallet.ValidChainAddress(s, "") == nil {
			  return s, nil
		  }
		  // 未知链名仍收下但要求字母数字，避免把脚本当 chain。
		   for _, r := range s {
			   if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
				   return "", fmt.Errorf("%w: %q", ErrChain, s)
			   }
		  }
		  if len(s) > 16 {
			   return "", fmt.Errorf("%w: too long", ErrChain)
		   }
		  return s, nil
	   }
  }

   func Idem(s string) (string, error) {
	  s = Trim(s)
	  if s == "" {
		   return "", ErrIdem
	  }
	   if len(s) > 128 {
		  return "", fmt.Errorf("%w: too long", ErrIdem)
	  }
	  return s, nil
  }

  func Amount(s string) (string, error) {
	  amt, err := money.Parse(Trim(s))
	  if err != nil {
		  return "", err
	   }
	   return amt.String(), nil
  }

// Address 链上地址。空地址直接拒，避免筛子空跑。
   func Address(chain, addr string) (string, error) {
	   addr = Trim(addr)
	  if addr == "" {
		  return "", ErrAddr
	  }
	  ch, err := Chain(chain)
	  if err != nil {
		   return "", err
	   }
	   if err := wallet.ValidChainAddress(ch,
		  addr); err != nil {
		  return "", err
	   }
	  return addr, nil
   }

   func Book(s string) (string, error) {
	  s = strings.ToLower(Trim(s))
	  switch s {
	   case "funding", "spot", "trading", "margin", "isolated", "futures", "earn", "wallet":
		   if s == "spot" {
			  return "funding", nil
		   }
		   return s, nil
	   default:
		  return "", fmt.Errorf("unknown book %q", s)
	  }
  }
