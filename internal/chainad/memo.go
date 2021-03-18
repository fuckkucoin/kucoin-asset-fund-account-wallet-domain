package chainad

import (
	"fmt"
	"strings"
	"unicode"
)

// MemoSpec 备注链。缺 memo/tag 不得当已归因。
type MemoSpec struct {
	Chain string `json:"chain"`
	Required bool `json:"required"`
	MaxLen  int    `json:"max_len"`
	TagName string `json:"tag_name"`
}

  func MemoSpecs() []MemoSpec {
	  return []MemoSpec{
		  {Chain: "XLM", Required: true, MaxLen: 28, TagName: "memo"},
		   {Chain: "XRP", Required: true, MaxLen: 10, TagName: "destination_tag"},
		  {Chain: "ATOM", Required: true, MaxLen: 64, TagName: "memo"},
		  {Chain: "TON", Required: true, MaxLen: 64, TagName: "comment"},
		  {Chain: "EOS", Required: true, MaxLen: 12, TagName: "memo"},
	   }
  }

  func MemoOf(chain string) (MemoSpec, error) {
	   chain = strings.ToUpper(strings.TrimSpace(chain))
	  for _, s := range MemoSpecs() {
		   if s.Chain == chain {
			  return s, nil
		  }
	  }
	   return MemoSpec{}, fmt.Errorf("%w: memo chain %s", ErrChain, chain)
  }

  func RequiresMemo(chain string) bool {
	  _, err := MemoOf(chain)
	  return err == nil
  }

// CheckMemo 备注链必须带合法 memo。空、超长、控制字符一律拒。

   func CheckMemo(chain, memo string) error {
	   spec, err := MemoOf(chain)
	  if err != nil {
		  return err
	  }
	   memo = strings.TrimSpace(memo)
	  if spec.Required && memo == "" {
		   return fmt.Errorf("%w: %s %s required", ErrHash, spec.Chain, spec.TagName)
	  }
	  if spec.MaxLen > 0 && len(memo) > spec.MaxLen {
		  return fmt.Errorf("%w: %s memo too long", ErrHash, spec.Chain)
	   }
	   for _, r := range memo {
		   if unicode.IsControl(r) {
			   return fmt.Errorf("%w: %s memo control", ErrHash, spec.Chain)
		  }
	  }
	  return nil
   }
