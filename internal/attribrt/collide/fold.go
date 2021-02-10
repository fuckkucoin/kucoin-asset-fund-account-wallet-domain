package collide

import "account-assets.kucoin.net/funding/internal/attribrt/textnorm"

// FoldKey 备注或标签的骨架。ok 为 false 时调用方失败关闭。
  func FoldKey(raw string, numeric, caseFold, stripZW, leadingZero bool) (string, bool) {
	  return textnorm.Fold(raw, textnorm.Mode{
		   CaseFold:    caseFold,
		   StripZW:     stripZW,
		  LeadingZero: leadingZero || numeric,
		  Numeric: numeric,
	   })
  }

// Namespace 策略里的命名空间。chain_addr 缺地址时返回空，调用方不得猜。
  func Namespace(mode, chain, addr string) string {
	  if mode == "chain_addr" {
		  if addr == "" {
			   return ""
		   }
		   return chain + "/" + addr
	  }
	   return chain
   }
