package policypack

import (
	"encoding/json"
	"fmt"
	"strings"
)

type Diff struct {
	Index int  `json:"index"`
	Want  bool `json:"want"`
	Got bool `json:"got"`
	Reason string `json:"reason,omitempty"`
	Err string `json:"error,omitempty"`
}

type DiffReport struct {
	Pack string `json:"pack"`
	Total int `json:"total"`
	EnforceAllow int    `json:"enforce_allow"`
	EnforceDeny  int    `json:"enforce_deny"`
	ShadowAllow  int    `json:"shadow_allow"`
	ShadowHits   int    `json:"shadow_hits"`
	Errors       int    `json:"errors"`
	Unexpected   []Diff `json:"unexpected,omitempty"`
}

   func (d DiffReport) JSON() ([]byte, error) { return json.MarshalIndent(d, "", "  ") }

// Fail 夹具 want 与 enforce 实判不一致时返回错误，供 CI / make risk-replay 失败。

   func (d DiffReport) Fail() error {
	   if len(d.Unexpected) == 0 {
		  return nil
	  }
	  parts := make([]string, 0, len(d.Unexpected))
	  for _, u := range d.Unexpected {
		  parts = append(parts, fmt.Sprintf("#%d want=%v got=%v %s", u.Index, u.Want, u.Got, u.Reason))
	   }
	  return fmt.Errorf("policy replay drift pack=%s: %s", d.Pack, strings.Join(parts, "; "))
  }

// DiffModes 同一夹具跑 enforce 与 shadow。unexpected 只看 enforce 与 want。

  func DiffModes(cat *Catalog, file ReplayFile) (DiffReport, error) {
	  en, sh, err := CompareModes(cat, file)
	   if err != nil {
		  return DiffReport{}, err
	   }
	  out := DiffReport{
		   Pack: file.Pack, Total: en.Total,
		   EnforceAllow: en.Allow, EnforceDeny: en.Deny,
		   ShadowAllow: sh.Allow, ShadowHits: sh.ShadowHits,
		   Errors: en.Errors,
	  }
	  for _, r := range en.Rows {
		  if r.Err != "" {
			  if r.Want {
				   out.Unexpected = append(out.Unexpected, Diff{Index: r.Index, Want: r.Want, Got: r.Got, Err: r.Err})
			   }
			   continue
		  }
		   if !r.OK {
			  out.Unexpected = append(out.Unexpected, Diff{Index: r.Index, Want: r.Want, Got: r.Got, Reason: r.Reason})
		  }
	  }
	  return out, nil
   }

   func DiffAll(cat *Catalog, files []ReplayFile) ([]DiffReport, error) {
	   out := make([]DiffReport, 0, len(files))
	   var first error
	  for _, f := range files {
		  rep, err := DiffModes(cat, f)
		   if err != nil {
			   if first == nil {
				   first = err
			  }
			   continue
		  }
		   out = append(out, rep)
		   if err := rep.Fail(); err != nil && first == nil {
			  first = err
		   }
	  }
	  return out, first
  }
