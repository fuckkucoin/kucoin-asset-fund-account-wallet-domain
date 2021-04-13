package depoccup

import (
	"strings"
	"time"
)

const (
	MaxRatioBPS  = 10000
	MaxWindowCap = 7 * 24 * time.Hour
	MinWindowCap = time.Second
)

   func NormAsset(s string) (string, error) {
	  s = strings.ToUpper(strings.TrimSpace(s))
	  if s == "*" {
		   return "*", nil
	   }
	  if s == "" || len(s) > 16 {
		   return "", ErrAsset
	   }
	   for _, r := range s {
		  if (r < 'A' || r > 'Z') && (r < '0' || r > '9') {
			  return "", ErrAsset
		  }
	  }
	  return s, nil
  }

   func NormChain(s string) (string, error) {
	  s = strings.ToUpper(strings.TrimSpace(s))
	  if s == "*" {
		  return "*", nil
	   }
	   if s == "" || len(s) > 16 {
		   return "", ErrChain
	   }
	   for _, r := range s {
		  if (r < 'A' || r > 'Z') && (r < '0' || r > '9') {
			  return "", ErrChain
		  }
	   }
	  return s, nil
  }

// FullNeed 与链族确认深度同值，只写入批次供释放条件展示。
// 延迟档和提前入账仍由 deposit.DelayBook 计算。
   func FullNeed(chain string) int {
	   switch strings.ToUpper(strings.TrimSpace(chain)) {
	  case "BTC":
		  return 3
	   case "TRON":
		  return 19
	   case "SOL":
		  return 32
	  case "XLM", "XRP", "ATOM", "TON", "EOS":
		  return 1
	  case "ETH", "EVM", "BASE", "ARB", "OP", "MATIC", "AVAX", "BSC", "POLYGON", "ARBITRUM", "OPTIMISM":
		   return 12
	   default:
		  return 12
	  }
   }

type chainWeight struct {
	AddBPS int
	AddWin time.Duration
}

type assetWeight struct {
	AddBPS int
	Class string
}

   func chainWeights() map[string]chainWeight {
	   return map[string]chainWeight{
		   "BTC":  {AddBPS: 0, AddWin: 0},
		  "ETH":  {AddBPS: 200, AddWin: 10 * time.Minute},
		  "BASE": {AddBPS: 300, AddWin: 15 * time.Minute},
		  "ARB":  {AddBPS: 300, AddWin: 15 * time.Minute},
		   "OP": {AddBPS: 350, AddWin: 15 * time.Minute},
		   "SOL":  {AddBPS: 400, AddWin: 20 * time.Minute},
		  "AVAX": {AddBPS: 450, AddWin: 20 * time.Minute},
		  "MATIC": {AddBPS: 450, AddWin: 20 * time.Minute},
		  "TON":   {AddBPS: 500, AddWin: 30 * time.Minute},
		  "BSC":   {AddBPS: 700, AddWin: 40 * time.Minute},
		  "TRON":  {AddBPS: 800, AddWin: 45 * time.Minute},
		   "XRP":   {AddBPS: 600, AddWin: 30 * time.Minute},
		   "XLM": {AddBPS: 600, AddWin: 30 * time.Minute},
		  "ATOM": {AddBPS: 650, AddWin: 30 * time.Minute},
		  "EOS": {AddBPS: 700, AddWin: 40 * time.Minute},
	  }
   }

  func assetWeights() map[string]assetWeight {
	   return map[string]assetWeight{
		  "BTC": {AddBPS: 0, Class: "major"},
		   "ETH": {AddBPS: 100, Class: "major"},
		   "USDT": {AddBPS: 500, Class: "stable"},
		  "USDC": {AddBPS: 400, Class: "stable"},
		   "DAI": {AddBPS: 450, Class: "stable"},
		  "SOL": {AddBPS: 350, Class: "alt"},
		  "TRX": {AddBPS: 600, Class: "alt"},
		  "TON":  {AddBPS: 550, Class: "alt"},
		  "XRP":  {AddBPS: 500, Class: "alt"},
		  "DOGE": {AddBPS: 700, Class: "alt"},
		   "BNB":   {AddBPS: 500, Class: "alt"},
		  "MATIC": {AddBPS: 550, Class: "alt"},
		  "AVAX":  {AddBPS: 500, Class: "alt"},
		   "ATOM": {AddBPS: 550, Class: "alt"},
		  "SHIB": {AddBPS: 900, Class: "meme"},
		   "PEPE": {AddBPS: 900, Class: "meme"},
	   }
   }

   func kycBase() map[string]struct {
	   BPS int
	   Win time.Duration
   } {
	return map[string]struct {
		BPS int
		Win time.Duration
	}{
		KYCAdvanced: {BPS: 500, Win: 30 * time.Minute},
		KYCBasic:    {BPS: 2000, Win: 2 * time.Hour},
		KYCNone: {BPS: 6000,
			Win: 12 * time.Hour},
	}
}

   func sourceBase() map[Source]struct {
	  BPS int
	   Mul int
   } {
	return map[Source]struct {
		BPS int
		Mul int
	}{
		SourceSelf:     {BPS: 0, Mul: 1},
		SourceVASP:     {BPS: 200, Mul: 1},
		SourceExchange: {BPS: 400, Mul: 2},
		SourceContract: {BPS: 800, Mul: 2},
		SourceUnknown:  {BPS: 1500, Mul: 4},
		SourceMixer:    {BPS: 10000, Mul: 8},
	}
}

   func clampBPS(n int) int {
	   if n < 0 {
		  return 0
	  }
	   if n > MaxRatioBPS {
		   return MaxRatioBPS
	  }
	   return n
  }

   func clampWindow(d time.Duration) time.Duration {
	   if d < MinWindowCap {
		  return MinWindowCap
	  }
	   if d > MaxWindowCap {
		  return MaxWindowCap
	   }
	  return d
  }

  func KnownChains() []string {
	  w := chainWeights()
	   out := make([]string, 0, len(w))
	   for k := range w {
		  out = append(out, k)
	   }
	  return out
   }

   func KnownAssets() []string {
	  w := assetWeights()
	   out := make([]string, 0, len(w))
	  for k := range w {
		  out = append(out, k)
	  }
	  return out
   }
