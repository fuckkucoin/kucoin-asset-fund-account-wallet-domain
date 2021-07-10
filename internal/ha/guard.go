package ha

import (
	"strings"
)

// RegionLabel 把 a/sg 和 b/tyo 归一成 ap-southeast-1 / ap-northeast-1。
   func RegionLabel(region string) string {
	  switch strings.ToLower(strings.TrimSpace(region)) {
	   case "a", "sg", "ap-southeast-1", "singapore":
		  return "sg-ap-southeast-1"
	  case "b", "tyo", "ap-northeast-1", "tokyo":
		  return "tyo-ap-northeast-1"
	   default:
		  return strings.TrimSpace(region)
	   }
  }

   func RegionCIDR(region string) string {
	  switch RegionLabel(region) {
	  case "sg-ap-southeast-1":
		   return "10.18.0.0/16"
	   case "tyo-ap-northeast-1":
		  return "10.19.0.0/16"
	  default:
		   return ""
	  }
  }

// GuardWrite 脑裂或无主时不得落账。fn 只在持主后执行。

   func (p *Pair) GuardWrite(fn func() error) error {
	  if err := p.RequireWrite(); err != nil {
		  return err
	   }
	   if fn == nil {
		   return nil
	   }
	   return fn()
  }
