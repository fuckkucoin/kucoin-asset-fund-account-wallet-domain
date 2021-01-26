package addrpool

import "strings"

// SharedNamespace 返回某链已登记的共享充值地址。没有登记时 ok 为 false，调用方不得派生。
  func SharedNamespace(p *SharedMemoPool, chain string) (string, bool) {
	  if p == nil {
		   return "", false
	  }
	  addr, ok := p.Address(strings.ToUpper(strings.TrimSpace(chain)))
	   if !ok || strings.TrimSpace(addr) == "" {
		  return "", false
	  }
	  return addr, true
   }
