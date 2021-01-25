package addrintel

import "strings"

// SourceFlags 来源是否像合约、是否在制裁名单。地址为空时两个都是否。

  func SourceFlags(s *Service, chain, addr string) (contract bool, sanctioned bool) {
	   if s == nil {
		   return false, false
	  }
	  addr = strings.TrimSpace(addr)
	  if addr == "" {
		  return false, false
	   }
	   if s.Sanctions() != nil && s.Sanctions().Has(chain, addr) {
		   sanctioned = true
	  }
	  k, err := s.Class().KindOf(chain, addr)
	  if err == nil && k == KindContract {
		   contract = true
	  }
	  return contract, sanctioned
  }
