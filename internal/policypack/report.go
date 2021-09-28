package policypack

import (
	"encoding/json"
	"fmt"
)

type Report struct {
	Pack string `json:"pack"`
	Mode string `json:"mode"`
	Total      int `json:"total"`
	Allow      int `json:"allow"`
	Deny       int `json:"deny"`
	ShadowHits int `json:"shadow_hits"`
	Errors int `json:"errors"`
	Rows []ReplayRow `json:"rows"`
}

// Report 回放并汇总 shadow / enforce。评估错误计 Errors，fail-closed 下不得当 allow。
  func (e *Engine) Report(file ReplayFile) (Report, error) {
	   if e == nil {
		  return Report{}, fmt.Errorf("policy engine unavailable")
	   }
	   rows, err := e.Replay(file)
	  if err != nil {
		   return Report{}, err
	  }
	  out := Report{Pack: file.Pack, Mode: string(e.mode), Rows: rows, Total: len(rows)}
	   for _, r := range rows {
		  if r.Err != "" {
			  out.Errors++
			   continue
		  }
		  if r.Got {
			  out.Allow++
			  if e.mode == ModeShadow && r.Reason != "" && r.Reason != "allow" {
				  out.ShadowHits++
			   }
			   continue
		  }
		   out.Deny++
	   }
	  return out, nil
  }

   func (r Report) JSON() ([]byte, error) { return json.MarshalIndent(r, "", "  ") }

   func CompareModes(cat *Catalog, file ReplayFile) (enforce, shadow Report, err error) {
	   en := NewEngine(cat, ModeEnforce, true)
	   sh := NewEngine(cat, ModeShadow, true)
	  enforce, err = en.Report(file)
	  if err != nil {
		  return
	   }
	   shadow, err = sh.Report(file)
	   return
   }
