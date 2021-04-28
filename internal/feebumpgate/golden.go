package feebumpgate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
)

// GoldenCase testdata 结构。
type GoldenCase struct {
	Scenario string `json:"scenario"`
	TransferID string `json:"transfer_id"`
	Chain string `json:"chain"`
	To string `json:"to"`
	Amount   string `json:"amount"`
	Asset    string `json:"asset"`
	FirstFee string `json:"first_fee"`
	SecondFee string `json:"second_fee"`
	DestinationTag    string `json:"destination_tag"`
	PrevBroadcastHash string `json:"prev_broadcast_hash"`
	QuotedPrevHash   string `json:"quoted_prev_hash"`
	Actor            string `json:"actor"`
	InjectRetireFail bool   `json:"inject_retire_fail"`
	Expected         struct {
		BlocksFurtherSign bool `json:"blocks_further_sign"`
	} `json:"expected"`
	Invariants []string `json:"invariants"`
}

  func testdataDir() string {
	  _, file, _, _ := runtime.Caller(0)
	   return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "testdata", "feebumpgate"))
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

  func (g GoldenCase) firstReq() SignWithGenReq {
	  return SignWithGenReq{
		  TransferID: g.TransferID, Chain: g.Chain, To: g.To, Amount: g.Amount,
		  Asset: g.Asset, NewFee: g.FirstFee, DestinationTag: g.DestinationTag,
		   Actor: g.Actor, IdempotencyKey: g.TransferID + ":1",
	   }
   }

   func (g GoldenCase) secondReq(prevHash string) SignWithGenReq {
	  h := g.QuotedPrevHash
	  if h == "" {
		   h = prevHash
	   }
	  if g.PrevBroadcastHash != "" && g.QuotedPrevHash == "" && g.Scenario == "prev_hash_mismatch" {
		   h = g.PrevBroadcastHash
	   }
	   return SignWithGenReq{
		   TransferID: g.TransferID, Chain: g.Chain, To: g.To, Amount: g.Amount,
		   Asset: g.Asset, NewFee: g.SecondFee, DestinationTag: g.DestinationTag,
		  PrevBroadcastHash: h, Actor: g.Actor, IdempotencyKey: g.TransferID + ":2",
	  }
   }
