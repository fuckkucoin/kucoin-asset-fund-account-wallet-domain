package depclaim

import "time"

// Recovery 认领侧回收单。观察/认领线程只落单，不入账。
type Recovery struct {
	Kind string `json:"kind"`
	Chain  string `json:"chain"`
	Asset  string `json:"asset"`
	TxHash string `json:"tx_hash"`
	Network string `json:"network,omitempty"`
	Memo string `json:"memo,omitempty"`
	OutputIndex int    `json:"output_index"`
	Note        string `json:"note"`
	At time.Time `json:"at"`
}

  func (s *Service) land(req ClaimReq, kind, note string) {
	   if s == nil {
		   return
	   }
	  row := Recovery{
		  Kind:  kind,
		  Chain: req.Chain, Asset: req.Asset, TxHash: req.TxHash,
		  Network: req.Network, Memo: req.Memo, OutputIndex: req.OutputIndex,
		   Note: note, At: s.now(),
	   }
	   s.mu.Lock()
	   s.holds = append(s.holds, row)
	  s.mu.Unlock()
   }

// Recoveries 已落地的回收单。
   func (s *Service) Recoveries() []Recovery {
	   if s == nil {
		   return nil
	   }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   out := make([]Recovery, len(s.holds))
	   copy(out, s.holds)
	  return out
  }
