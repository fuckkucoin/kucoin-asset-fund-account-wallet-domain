package riskpack

import (
	"strings"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/pkg/money"
)

type ReplayIn struct {
	UserID string `json:"user_id"`
	Asset string `json:"asset"`
	Amount string   `json:"amount"`
	Chain  string   `json:"chain"`
	Dest   string   `json:"dest,omitempty"`
	Want   Decision `json:"want"`
	Release bool `json:"release,omitempty"`
}

type ReplayOut struct {
	Index int `json:"index"`
	Got Decision `json:"got"`
	Want   Decision `json:"want"`
	OK     bool     `json:"ok"`
	Reason string   `json:"reason,omitempty"`
	Escalate string `json:"escalate_id,omitempty"`
	Err string `json:"error,omitempty"`
}

type ReplaySum struct {
	Total int `json:"total"`
	Matched int `json:"matched"`
	Drift int `json:"drift"`
	Closed int         `json:"fail_closed"`
	Rows   []ReplayOut `json:"rows"`
}

// Replay 按历史单回放敞口/簇/相关闸。金额坏失败即关；Release 在评估后放占用。

   func (e *Engine) Replay(rows []ReplayIn) (ReplaySum, error) {
	   if e == nil {
		  return ReplaySum{},
			  ErrUnavailable
	   }
	   out := ReplaySum{Total: len(rows)}
	   for i, in := range rows {
		  row := ReplayOut{Index: i, Want: in.Want}
		  if _, err := validate.UserID(in.UserID); err != nil {
			  row.Err = err.Error()
			   out.Closed++
			  out.Rows = append(out.Rows, row)
			  continue
		   }
		  if _, err := money.Parse(in.Amount); err != nil {
			  row.Err = err.Error()
			   out.Closed++
			  out.Rows = append(out.Rows, row)
			   continue
		  }
		   got, err := e.Evaluate(Request{
			   UserID: in.UserID,
			  Asset:  in.Asset, Amount: in.Amount,
			   Chain: in.Chain, Dest: in.Dest,
		  })
		  if err != nil {
			  row.Err = err.Error()
			  out.Closed++
			  out.Rows = append(out.Rows, row)
			  continue
		   }
		  row.Got = got.Decision
		   row.Reason = got.Reason
		   row.Escalate = got.Escalate
		  row.OK = got.Decision == in.Want
		   if row.OK {
			   out.Matched++
		   } else {
			   out.Drift++
		   }
		  if in.Release || got.Decision != Allow {
			   _ = e.Release(Request{UserID: in.UserID,
				  Asset: in.Asset, Amount: in.Amount})
		  }
		  out.Rows = append(out.Rows, row)
	   }
	  return out, nil
   }

   func NormalizeDecision(s string) Decision {
	  switch strings.ToLower(strings.TrimSpace(s)) {
	  case string(Hold):
		  return Hold
	   case string(Deny):
		   return Deny
	  default:
		  return Allow
	  }
   }
