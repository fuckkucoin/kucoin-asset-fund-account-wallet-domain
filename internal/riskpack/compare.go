package riskpack

import (
	"encoding/json"
	"fmt"
	"strings"
)

type ModeRow struct {
	Index int      `json:"index"`
	Want  Decision `json:"want"`
	Enforce Decision `json:"enforce"`
	Shadow Decision `json:"shadow"`
	Reason string   `json:"reason,omitempty"`
	OK     bool     `json:"ok"`
	Closed bool   `json:"fail_closed,omitempty"`
	Err    string `json:"error,omitempty"`
}

type CompareReport struct {
	Total   int `json:"total"`
	Matched int `json:"matched"`
	Drift   int `json:"drift"`
	Closed     int       `json:"fail_closed"`
	ShadowDeny int       `json:"shadow_would_deny"`
	Rows       []ModeRow `json:"rows"`
}

  func (r CompareReport) JSON() ([]byte, error) { return json.MarshalIndent(r, "", "  ") }

// Fail 夹具 want 与 enforce 实判不一致时失败。shadow 不得改变 enforce 结论。
   func (r CompareReport) Fail() error {
	  if r.Drift == 0 {
		  return nil
	   }
	  parts := make([]string, 0, r.Drift)
	   for _, row := range r.Rows {
		  if row.OK || row.Closed {
			  continue
		  }
		  parts = append(parts, fmt.Sprintf("#%d want=%s enforce=%s", row.Index, row.Want, row.Enforce))
	   }
	   if len(parts) == 0 {
		  return fmt.Errorf("risk replay drift=%d", r.Drift)
	  }
	   return fmt.Errorf("risk replay drift: %s", strings.Join(parts, "; "))
   }

// CompareReplay enforce 必须命中 want；shadow 把 Hold/Deny 记为允许但保留原判。

   func (e *Engine) CompareReplay(rows []ReplayIn) (CompareReport, error) {
	  if e == nil {
		   return CompareReport{}, ErrUnavailable
	  }
	   sum, err := e.Replay(rows)
	   if err != nil {
		  return CompareReport{}, err
	  }
	   out := CompareReport{Total: sum.Total, Matched: sum.Matched, Drift: sum.Drift, Closed: sum.Closed, Rows: make([]ModeRow, 0, len(sum.Rows))}
	   for _, r := range sum.Rows {
		  row := ModeRow{
			   Index: r.Index, Want: r.Want, Enforce: r.Got, Reason: r.Reason,
			   OK: r.OK, Err: r.Err, Closed: r.Err != "",
		   }
		   if r.Err != "" {
			   row.Shadow = Allow
		   } else if r.Got == Allow {
			  row.Shadow = Allow
		  } else {
			   row.Shadow = Allow
			   out.ShadowDeny++
		   }
		  out.Rows = append(out.Rows, row)
	   }
	   return out, nil
   }
