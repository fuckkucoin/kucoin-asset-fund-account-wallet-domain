package subkey

// CIDRLadderEntry 推荐网段。

type CIDRLadderEntry struct {
	Region string `json:"region"`
	CIDR   string `json:"cidr"`
	Label  string `json:"label"`
}

var cidrLadder = []CIDRLadderEntry{
	{Region: "SG", CIDR: "10.18.0.0/16", Label: "新加坡"},
	{Region: "TK", CIDR: "10.19.0.0/16", Label: "东京"},
	{Region: "SG", CIDR: "203.0.113.0/24", Label: "办公出口示例"},
	{Region: "TK", CIDR: "198.51.100.0/24", Label: "办公出口示例"},
}

// CIDRLadder 返回推荐网段表。

   func CIDRLadder() []CIDRLadderEntry {
	   out := make([]CIDRLadderEntry, len(cidrLadder))
	   copy(out, cidrLadder)
	   return out
   }

// SuggestWhitelistForRegion 按区域拼接推荐 CIDR（逗号分隔）。
   func SuggestWhitelistForRegion(region string) string {
	  parts := make([]string, 0)
	   for _, e := range cidrLadder {
		   if e.Region == region {
			  parts = append(parts, e.CIDR)
		   }
	  }
	  return joinComma(parts)
  }

  func joinComma(parts []string) string {
	  if len(parts) == 0 {
		   return ""
	  }
	   s := parts[0]
	  for i := 1; i < len(parts); i++ {
		  s += "," + parts[i]
	   }
	   return s
  }

// ValidateAgainstLadder 校验白名单是否仅包含阶梯表内 CIDR（可选严模式）。
  func ValidateAgainstLadder(raw string, strict bool) error {
	  if !strict || raw == "" {
		   return ValidateIPWhitelistString(raw)
	   }
	   rules, err := ParseIPWhitelist(raw)
	   if err != nil {
		   return err
	  }
	   allowed := map[string]bool{}
	   for _, e := range cidrLadder {
		  allowed[e.CIDR] = true
	  }
	  for _, r := range rules {
		   if !r.IsCIDR || !allowed[r.Raw] {
			  return ErrBadIP
		   }
	  }
	  return nil
  }

// LadderHealth 探针。

  func LadderHealth() map[string]any {
	   return map[string]any{"entries": len(cidrLadder), "regions": []string{"SG", "TK"}}
  }
