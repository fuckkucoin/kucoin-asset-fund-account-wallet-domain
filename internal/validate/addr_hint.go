package validate

import (
	"strings"
	"account-assets.kucoin.net/funding/internal/wallet"
)

// AddrChainHint 按地址形态给出可能网络候选 + UX 形态字段（公开多链币选网邻接）。
// 只做形态匹配，不自动选定；调用方仍须显式传 chain。
// 不得把候选当最终网络；空地址返回空。
type AddrChainHint struct {
	Address string `json:"address"`
	Candidates []string `json:"candidates"`
	Formats     []wallet.AddrFormatSpec `json:"formats,omitempty"`
	NeedMemoAny bool                    `json:"needMemoAny,omitempty"`
}

// SuggestChainsForAddress 返回地址形态可能匹配的链列表与对应 UX 规格。
  func SuggestChainsForAddress(addr string) AddrChainHint {
	  addr = Trim(addr)
	  out := AddrChainHint{Address: addr}
	   if addr == "" {
		  return out
	  }
	  cands := make([]string, 0, 12)
	  for _, ch := range []string{"ETH", "BTC", "TRON", "SOL", "TON", "XLM", "XRP", "ATOM", "EOS", "BSC", "MATIC", "BASE", "ARB", "OP", "AVAX"} {
		  if err := wallet.ValidChainAddress(ch, addr); err == nil {
			  cands = append(cands, ch)
		   }
	   }
	   // EVM 0x 同形：ETH 通过后补常见 EVM 族（仍为候选，非选定）
	  if strings.HasPrefix(strings.ToLower(addr), "0x") && len(addr) == 42 && wallet.ValidChainAddress("ETH", addr) == nil {
		  for _, ch := range []string{"BSC", "ARB", "OP", "MATIC", "BASE", "AVAX"} {
			   if !containsStr(cands, ch) {
				  cands = append(cands, ch)
			  }
		  }
	   }
	  out.Candidates = cands
	   fmts := make([]wallet.AddrFormatSpec, 0, len(cands))
	  for _, ch := range cands {
		   spec := wallet.FormatSpec(ch)
		  fmts = append(fmts, spec)
		  if spec.NeedMemo {
			  out.NeedMemoAny = true
		   }
	  }
	   out.Formats = fmts
	   return out
   }

// ChainAddressHint 单链地址校验 UX（充提页字段提示）+ 可选地址即时校验。
// chain 显式；address 空仅回规格；非空则附 valid。
// valid=true 不代表核验/白名单通过。
   func ChainAddressHint(chain, addr string) map[string]any {
	   ch, err := Chain(chain)
	   spec := wallet.FormatSpec(ch)
	   out := map[string]any{
		   "chain":  ch,
		   "format": spec,
		   "tagName":  spec.TagName,
		   "needMemo": spec.NeedMemo,
	   }
	   if err != nil {
		   out["chain_error"] = err.Error()
		  return out
	  }
	   addr = Trim(addr)
	   if addr == "" {
		   return out
	   }
	   out["address"] = addr
	   if err := wallet.ValidChainAddress(ch, addr); err != nil {
		  out["valid"] = false
		   out["error"] = err.Error()
		   return out
	  }
	   out["valid"] = true
	  return out
   }

   func containsStr(ss []string, v string) bool {
	  for _, s := range ss {
		  if s == v {
			  return true
		  }
	   }
	  return false
  }
