// 提现和白名单的地址形态校验。
// 作者: Likeo
// 日期: 2021.01.14
package wallet

import (
	"fmt"
	"strings"
	"unicode"
	"github.com/btcsuite/btcd/btcutil/base58"
)

// ValidChainAddress
// 按指定链主网规则校验目标地址字符串。
// 去空白后做链别形态判定；FamMEMO 走 hardenFamMemoAddress。
// 空地址或未知链应失败；通过后仍须叠加白名单/制裁等业务闸。
   func ValidChainAddress(chain, addr string) error {
	  addr = strings.TrimSpace(addr)
	  if addr == "" {
		  return fmt.Errorf("address required")
	  }
	   switch {
	  case chain == "EVM" || IsEVM(chain):
		   if !strings.HasPrefix(strings.ToLower(addr), "0x") || len(addr) != 42 {
			   return fmt.Errorf("evm length")
		  }
		  for _, r := range addr[2:] {
			  if !unicode.Is(unicode.ASCII_Hex_Digit, r) {
				  return fmt.Errorf("evm hex")
			   }
		   }
	   case chain == "BTC":
		  low := strings.ToLower(addr)
		   if strings.HasPrefix(low, "tb1") || strings.HasPrefix(low, "bcrt") {
			  return fmt.Errorf("btc testnet")
		   }
		  if !strings.HasPrefix(low, "bc1") && !strings.HasPrefix(addr, "1") && !strings.HasPrefix(addr, "3") {
			  return fmt.Errorf("btc prefix")
		  }
		   if len(addr) < 26 || len(addr) > 90 {
			   return fmt.Errorf("btc length")
		  }
	  case chain == "SOL":
		  if len(addr) < 32 || len(addr) > 44 {
			  return fmt.Errorf("sol length")
		   }
		  if raw := base58.Decode(addr); len(raw) != 32 {
			  return fmt.Errorf("sol base58")
		  }
	   case chain == "TRON":
		  if !strings.HasPrefix(addr, "T") || len(addr) < 30 || len(addr) > 36 {
			   return fmt.Errorf("tron format")
		  }
		   if _, _, err := base58.CheckDecode(addr); err != nil {
			  return fmt.Errorf("tron checksum")
		   }
	  case RequiresFamMemo(chain):
		  return hardenFamMemoAddress(chain, addr)
	   default:
		   return fmt.Errorf("unknown chain %s", chain)
	  }
	   return nil
   }

// hardenFamMemoAddress FamMEMO 链地址形态深化（共享池 Register / 出金簿 / 校验 API）。
// XRP base58check、XLM 定长 base32、ATOM bech32 HRP、EOS 字符集、TON 前缀与长度。
// 深化失败须拒；不得因宽松前缀放过网/错链地址入共享池。
   func hardenFamMemoAddress(chain, addr string) error {
	   chain = strings.ToUpper(strings.TrimSpace(chain))
	   switch chain {
	  case "TON":
		  if !strings.HasPrefix(addr, "EQ") && !strings.HasPrefix(addr, "UQ") {
			  return fmt.Errorf("ton prefix")
		   }
		  // 友好地址常见 48；放宽上下各 2 防编码变体
		   if len(addr) < 46 || len(addr) > 50 {
			  return fmt.Errorf("ton length")
		   }
		   for _, r := range addr {
			  if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
				   continue
			  }
			   return fmt.Errorf("ton charset")
		   }
	  case "XLM":
		   if (!strings.HasPrefix(addr, "G") && !strings.HasPrefix(addr, "M")) || len(addr) != 56 {
			  return fmt.Errorf("xlm format")
		  }
		  for _, r := range addr {
			   // Stellar base32：A-Z / 2-7
			  if (r >= 'A' && r <= 'Z') || (r >= '2' && r <= '7') {
				   continue
			  }
			   return fmt.Errorf("xlm base32")
		   }
	   case "XRP":
		   if strings.HasPrefix(addr, "X") {
			   if len(addr) < 25 || len(addr) > 64 {
				  return fmt.Errorf("xrp x-address length")
			   }
			   return nil
		   }
		   if !strings.HasPrefix(addr, "r") || len(addr) < 25 || len(addr) > 35 {
			  return fmt.Errorf("xrp format")
		   }
		   if _, _, err := base58.CheckDecode(addr); err != nil {
			  return fmt.Errorf("xrp checksum")
		  }
	  case "ATOM":
		   low := strings.ToLower(addr)
		  if !strings.HasPrefix(low, "cosmos1") {
			   return fmt.Errorf("atom hrp")
		  }
		   if len(addr) < 39 || len(addr) > 90 {
			  return fmt.Errorf("atom length")
		   }
		  for i, r := range low {
			  if i < len("cosmos1") {
				  continue
			  }
			   if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
				   continue
			  }
			   return fmt.Errorf("atom bech32")
		   }
	   case "EOS":
		   if len(addr) < 1 || len(addr) > 12 {
			  return fmt.Errorf("eos account")
		  }
		  if strings.HasPrefix(addr, ".") || strings.HasSuffix(addr, ".") || strings.Contains(addr, "..") {
			  return fmt.Errorf("eos dots")
		   }
		   for _, r := range addr {
			  if !((r >= 'a' && r <= 'z') || (r >= '1' && r <= '5') || r == '.') {
				   return fmt.Errorf("eos charset")
			   }
		  }
	  default:
		  return fmt.Errorf("unknown fam memo chain %s", chain)
	  }
	  return nil
  }

// ValidFamMemoTag 校验 FamMEMO 备注/Tag 形态（出金簿 / 充值绑归因）。
// XRP 须 uint32 十进制；其他备注限长与可见字符。
// 空备注在 RequiresFamMemo 链上应失败关闭；不得伪造默认 memo。
  func ValidFamMemoTag(chain, memo string) error {
	   chain = strings.ToUpper(strings.TrimSpace(chain))
	  memo = strings.TrimSpace(memo)
	  if !RequiresFamMemo(chain) {
		  if memo != "" {
			  return fmt.Errorf("memo not used on %s", chain)
		   }
		   return nil
	  }
	   if memo == "" {
		   return fmt.Errorf("memo required for %s", chain)
	   }
	   if len(memo) > 128 {
		  return fmt.Errorf("memo too long")
	  }
	   for _, r := range memo {
		  if unicode.IsControl(r) {
			  return fmt.Errorf("memo control")
		  }
	  }
	   switch FamMemoTagName(chain) {
	  case "destination_tag":
		  if len(memo) > 10 {
			   return fmt.Errorf("destination_tag length")
		   }
		   var n uint64
		  for _, r := range memo {
			   if r < '0' || r > '9' {
				   return fmt.Errorf("destination_tag digits")
			   }
			  n = n*10 + uint64(r-'0')
			  if n > 0xffffffff {
				   return fmt.Errorf("destination_tag overflow")
			   }
		   }
	  case "comment":
		   if len(memo) > 120 {
			   return fmt.Errorf("comment length")
		   }
	   case "memo":
		  if len(memo) > 28 && chain == "XLM" {
			  return fmt.Errorf("xlm memo length")
		  }
		   if len(memo) > 256 {
			   return fmt.Errorf("memo length")
		  }
	  }
	   return nil
  }
