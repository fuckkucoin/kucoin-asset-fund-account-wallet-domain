package memoeffectgate

import (
	"fmt"
	"strings"
)

type OutputRule struct {
	Chain string
	Index int
	Allowed  bool
	NeedMemo bool
	Reason string
}

   func BuildOutputRules() []OutputRule {
	   out := make([]OutputRule, 0, 400)
	  for _, ch := range AllChains() {
		   r, _ := LookupChain(ch)
		  max := r.MaxOutputs
		  if max > 8 {
			  max = 8
		   }
		  for i := -1; i <= max; i++ {
			   ok := OutputIndexAllowed(ch, i)
			  reason := "ok"
			   if i < 0 {
				  reason = "negative_index"
			   } else if i >= r.MaxOutputs {
				  reason = "index_overflow"
			   }
			   out = append(out, OutputRule{
				   Chain: ch, Index: i, Allowed: ok, NeedMemo: r.NeedMemo, Reason: reason,
			  })
		   }
	   }
	  return out
  }

  func CheckOutputRule(chain string, idx int, memo string) error {
	  chain = strings.ToUpper(strings.TrimSpace(chain))
	  if !OutputIndexAllowed(chain, idx) {
		  return ErrOutputKeyInvalid
	   }
	   if NeedMemo(chain) && strings.TrimSpace(memo) == "" {
		   return ErrBindMemoFailed
	   }
	  return nil
   }

   func FormatOutputRule(r OutputRule) string {
	   return fmt.Sprintf("%s#%d allowed=%v memo=%v %s", r.Chain, r.Index, r.Allowed, r.NeedMemo, r.Reason)
   }

   func CountAllowedRules() int {
	   n := 0
	   for _, r := range BuildOutputRules() {
		   if r.Allowed {
			  n++
		  }
	  }
	   return n
   }
