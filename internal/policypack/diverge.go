package policypack

import "fmt"

type Diverge struct {
	Index        int  `json:"index"`
	EnforceAllow bool `json:"enforce_allow"`
	ShadowAllow  bool `json:"shadow_allow"`
	Reason string `json:"reason,omitempty"`
}

type ModeReport struct {
	Enforce Report `json:"enforce"`
	Shadow     Report    `json:"shadow"`
	Diverge    []Diverge `json:"diverge"`
	Diverged   int       `json:"diverged"`
	FailClosed bool      `json:"fail_closed"`
}

// ReportModes 同时跑 enforce 与 shadow。缺包或评估错计入 Errors，不得当放行。

   func ReportModes(cat *Catalog, file ReplayFile) (ModeReport, error) {
	   out := ModeReport{FailClosed: true}
	   en, sh, err := CompareModes(cat, file)
	   if err != nil {
		   return out, err
	  }
	   out.Enforce, out.Shadow = en, sh
	   if len(en.Rows) != len(sh.Rows) {
		  return out, fmt.Errorf("policy mode row count drift")
	   }
	  for i := range en.Rows {
		   if en.Rows[i].Got == sh.Rows[i].Got && en.Rows[i].Err == sh.Rows[i].Err {
			   continue
		   }
		  out.Diverge = append(out.Diverge, Diverge{
			  Index:        i,
			   EnforceAllow: en.Rows[i].Got, ShadowAllow: sh.Rows[i].Got, Reason: sh.Rows[i].Reason,
		   })
	   }
	  out.Diverged = len(out.Diverge)
	   return out, nil
  }
