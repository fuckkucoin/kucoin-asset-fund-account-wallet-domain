package address

import (
	"fmt"
	"strings"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/internal/wallet"
)

// FormatRule 单链形态闸条。Required 表示写簿必须过 ValidChainAddress。

type FormatRule struct {
	wallet.AddrFormatSpec
	Required bool `json:"required"`
	Rule string `json:"rule"`
}

// FormatMatrix 已知资金链形态矩阵（写闸与 UX 同源）。

   func FormatMatrix() []FormatRule {
	   specs := wallet.FormatSpecs()
	   out := make([]FormatRule, 0, len(specs))
	   for _, spec := range specs {
		   out = append(out, FormatRule{
			  AddrFormatSpec: spec,
			  Required:       spec.Family != "UNKNOWN" && spec.Family != "",
			   Rule:           formatRuleName(spec),
		  })
	   }
	  return out
  }

  func formatRuleName(spec wallet.AddrFormatSpec) string {
	  switch spec.Family {
	  case "EVM":
		  return "evm-0x-40hex"
	  case "UTXO":
		  return "btc-bech32-or-base58"
	  case "SOL":
		   return "sol-base58-32"
	  case "TRON":
		  return "tron-base58check"
	  case "FamMEMO":
		   return "fam-memo-" + strings.ToLower(spec.TagName)
	  default:
		   return "unknown"
	  }
  }

// CheckFormatMatrix 按矩阵校验链地址与可选备注。
// 先 Chain/Address formal，再 FormatSpec 前缀与长度，最后 ValidChainAddress。FamMEMO 有 memo 必须过 ValidFamMemoTag；未知族失败关闭。

   func CheckFormatMatrix(chain, addr, memo string) (FormatRule, error) {
	   ch, err := validate.Chain(chain)
	  if err != nil {
		   return FormatRule{}, err
	   }
	   a, err := validate.Address(ch, addr)
	  if err != nil {
		  return FormatRule{}, err
	   }
	   spec := wallet.FormatSpec(ch)
	  rule := FormatRule{AddrFormatSpec: spec, Required: spec.Family != "UNKNOWN" && spec.Family != "", Rule: formatRuleName(spec)}
	   if spec.Family == "UNKNOWN" || spec.Family == "" {
		  return rule, fmt.Errorf("%w: unknown chain family", ledger.ErrAddressFormat)
	  }
	   if spec.MinLen > 0 && len(a) < spec.MinLen {
		   return rule, fmt.Errorf("%w: length", ledger.ErrAddressFormat)
	  }
	  if spec.MaxLen > 0 && len(a) > spec.MaxLen {
		  return rule, fmt.Errorf("%w: length", ledger.ErrAddressFormat)
	  }
	  if len(spec.Prefixes) > 0 && !prefixOK(a, spec.Prefixes) {
		   return rule, fmt.Errorf("%w: prefix", ledger.ErrAddressFormat)
	  }
	  if err := wallet.ValidChainAddress(ch, a); err != nil {
		  return rule, err
	   }
	  memo = strings.TrimSpace(memo)
	   if memo != "" {
		  if err := wallet.ValidFamMemoTag(ch, memo); err != nil {
			  return rule, err
		  }
	  }
	  return rule, nil
  }

  func prefixOK(addr string, prefixes []string) bool {
	  for _, p := range prefixes {
		   if p == "" {
			   continue
		   }
		  if strings.HasPrefix(addr, p) || strings.HasPrefix(strings.ToLower(addr), strings.ToLower(p)) {
			  return true
		   }
	   }
	  return false
   }
