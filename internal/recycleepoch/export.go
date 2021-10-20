package recycleepoch

import (
	"sort"
	"strings"
)

// ProofDigest 稳定可比较的证明摘要。
type ProofDigest struct {
	Kind        string `json:"kind"`
	FailClosed  bool   `json:"fail_closed"`
	Disposition string `json:"disposition"`
	PrevUser    string `json:"prev_user"`
	AttemptUser string `json:"attempt_user"`
	CurrentUser string `json:"current_user"`
	Generation int64 `json:"generation"`
	CarriedGen int64 `json:"carried_gen"`
	CreditOK   bool  `json:"credit_ok"`
}

  func DigestProof(p MiscreditProof) ProofDigest {
	   return ProofDigest{
		   Kind:        string(p.Kind),
		   FailClosed:  p.FailClosed,
		  Disposition: string(p.Disposition),
		  PrevUser:    p.PrevUser,
		   AttemptUser: p.AttemptUser,
		   CurrentUser: p.CurrentUser,
		   Generation: p.Generation,
		   CarriedGen: p.CarriedGen,
		  CreditOK: false,
	   }
  }

  func SortProofs(in []MiscreditProof) []MiscreditProof {
	  out := append([]MiscreditProof(nil), in...)
	  sort.Slice(out, func(i, j int) bool {
		   if out[i].Kind != out[j].Kind {
			  return out[i].Kind < out[j].Kind
		  }
		   if out[i].AttemptUser != out[j].AttemptUser {
			  return out[i].AttemptUser < out[j].AttemptUser
		   }
		   return out[i].OpenedAt.Before(out[j].OpenedAt)
	  })
	  return out
   }

  func SlotCanon(chain, address string) string {
	   return strings.ToUpper(strings.TrimSpace(chain)) + "|" + strings.ToLower(strings.TrimSpace(address))
  }

// ScenarioExpect golden 期望结构。

type ScenarioExpect struct {
	CreditPosted bool `json:"credit_posted"`
	Error string `json:"error,omitempty"`
	ProofKind string `json:"proof_kind,omitempty"`
	FailClosed bool   `json:"fail_closed"`
	EpochState string `json:"epoch_state,omitempty"`
	Generation int64  `json:"generation,omitempty"`
	BindUser   string `json:"bind_user,omitempty"`
	BindState  string `json:"bind_state,omitempty"`
}
