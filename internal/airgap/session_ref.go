package airgap

import "strings"

// SweepSessionRef 冷腿绑定的气隙编号。不是托管证书。
type SweepSessionRef struct {
	ID    string `json:"id"`
	Phase string `json:"phase"`
	Kind  string `json:"kind"`
	Asset string `json:"asset"`
	Amount      string `json:"amount"`
	PayloadHash string `json:"payload_hash"`
}

   func (s *Service) SweepRef(id string) (SweepSessionRef, error) {
	  if s == nil {
		   return SweepSessionRef{}, ErrUnavailable
	  }
	  row, err := s.Get(strings.TrimSpace(id))
	  if err != nil {
		  return SweepSessionRef{}, err
	  }
	  return SweepSessionRef{
		   ID:    row.ID,
		  Phase: string(row.Phase), Kind: row.Payload.Kind, Asset: row.Payload.Asset,
		  Amount: row.Payload.Amount, PayloadHash: row.PayloadHash,
	  }, nil
   }
