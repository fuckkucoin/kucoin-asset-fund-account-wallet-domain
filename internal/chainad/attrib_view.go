package chainad

import "strings"

// AttribView 给归因运行时抄走的只读快照。

type AttribView struct {
	Chain  string `json:"chain"`
	Family string `json:"family,omitempty"`
	Known bool `json:"known"`
	MemoRequired bool `json:"memo_required"`
	MemoMax int    `json:"memo_max,omitempty"`
	TagName string `json:"tag_name,omitempty"`
	Confirm int    `json:"confirm,omitempty"`
	Native string `json:"native,omitempty"`
}

// ReadAttribView 读链适配器里已经有的族和备注规格。未知链 Known 为 false。

  func ReadAttribView(chain string) AttribView {
	  chain = strings.ToUpper(strings.TrimSpace(chain))
	  out := AttribView{Chain: chain}
	  fam, err := FamilyOf(chain)
	   if err == nil {
		   out.Known = true
		   out.Family = string(fam)
		   out.Confirm = ConfirmNeed(fam)
		  out.Native = NativeAsset(fam)
	  }
	  if spec, e := MemoOf(chain); e == nil {
		  out.MemoRequired = spec.Required
		  out.MemoMax = spec.MaxLen
		  out.TagName = spec.TagName
	  }
	  return out
   }
