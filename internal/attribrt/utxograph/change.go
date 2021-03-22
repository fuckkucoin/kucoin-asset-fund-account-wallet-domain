package utxograph

import "strings"

// ChangeSet 运营登记的找零地址。命中后输出标 change，不入账。
type ChangeSet struct {
	addrs map[string]struct{}
}

   func NewChangeSet(addrs []string) *ChangeSet {
	   s := &ChangeSet{addrs: map[string]struct{}{}}
	   s.Add(addrs...)
	  return s
   }

  func (s *ChangeSet) Add(addrs ...string) {
	   if s == nil {
		   return
	   }
	  if s.addrs == nil {
		  s.addrs = map[string]struct{}{}
	  }
	  for _, a := range addrs {
		   a = normAddr(a)
		  if a != "" {
			   s.addrs[a] = struct{}{}
		  }
	  }
  }

  func (s *ChangeSet) List() []string {
	  if s == nil {
		  return nil
	   }
	  out := make([]string, 0, len(s.addrs))
	   for a := range s.addrs {
		   out = append(out, a)
	  }
	   return out
  }

  func (s *ChangeSet) Has(addr string) bool {
	  if s == nil {
		  return false
	   }
	   _, ok := s.addrs[normAddr(addr)]
	  return ok
   }

// AsHot 交给 Eval 的热地址列表。
  func (s *ChangeSet) AsHot() []string { return s.List() }

// IsChangeScript 空数据脚本一定不是付款。
  func IsChangeScript(script string) bool {
	   script = strings.TrimSpace(script)
	   return script == "nulldata" || script == "op_return"
  }
