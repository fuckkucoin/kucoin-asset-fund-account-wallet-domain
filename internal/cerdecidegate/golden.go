package cerdecidegate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
)

// GoldenCase testdata 结构。
type GoldenCase struct {
	Scenario string `json:"scenario"`
	BatchID  string `json:"batch_id"`
	Asset    string `json:"asset"`
	Chain  string `json:"chain"`
	Amount string `json:"amount"`
	Dest            string `json:"dest"`
	PolicyID        string `json:"policy_id"`
	QuotedThreshold int    `json:"quoted_threshold"`
	QuotedShares    int    `json:"quoted_shares"`
	LiveThreshold   int    `json:"live_threshold"`
	LiveShares int `json:"live_shares"`
	QuotedDestRisk string `json:"quoted_dest_risk"`
	LiveDestRisk   string `json:"live_dest_risk"`
	DestRisk       string `json:"dest_risk"`
	QuotedForceCold bool  `json:"quoted_force_cold"`
	LiveForceCold   *bool `json:"live_force_cold"`
	Required        *bool `json:"required"`
	Expected        struct {
		ExecError  string `json:"exec_error"`
		IssueError string `json:"issue_error"`
		NoLeaseExecError string `json:"no_lease_exec_error"`
		ProofKind        string `json:"proof_kind"`
		StateAfterAssert string `json:"state_after_assert"`
		StateAfterConsume string `json:"state_after_consume"`
	} `json:"expected"`
	Invariants []string `json:"invariants"`
}

  func testdataDir() string {
	   _, file, _, _ := runtime.Caller(0)
	   root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "testdata", "cerdecidegate"))
	   return root
  }

// LoadGolden 读单个夹具。

   func LoadGolden(name string) (GoldenCase, error) {
	   var g GoldenCase
	   b, err := os.ReadFile(filepath.Join(testdataDir(), name))
	  if err != nil {
		  return g, err
	   }
	   if err := json.Unmarshal(b, &g); err != nil {
		  return g, err
	   }
	   return g, nil
   }

  func (g GoldenCase) line() LineFact {
	  risk := g.QuotedDestRisk
	  if risk == "" {
		  risk = g.DestRisk
	   }
	   if risk == "" {
		  risk = "high"
	   }
	  return LineFact{
		   BatchID: g.BatchID, Asset: g.Asset, Chain: g.Chain, Amount: g.Amount,
		   Dest: g.Dest, DestRisk: risk, Side: "cold", Purpose: "withdraw",
	   }
   }

  func (g GoldenCase) pin() DecidePin {
	   req := true
	   if g.Required != nil {
		   req = *g.Required
	   }
	  th := g.QuotedThreshold
	  if th == 0 && req {
		   th = 3
	  }
	  sh := g.QuotedShares
	   if sh == 0 && req {
		   sh = 5
	   }
	  risk := g.QuotedDestRisk
	   if risk == "" {
		   risk = g.DestRisk
	   }
	   if risk == "" {
		  risk = "high"
	   }
	   return DecidePin{
		  Required: req,
		   PolicyID: g.PolicyID, Threshold: th, Shares: sh,
		  BindMode: "one", DestRisk: risk, Reason: "force:" + g.PolicyID,
	  }
   }
