package scoreleasegate

import "context"

type ObserveSnap struct {
	Gen   ScoreGeneration `json:"generation"`
	Stats GateStats       `json:"stats"`
	Open   int    `json:"open_leases"`
	Proofs int    `json:"open_proofs"`
	Mark   string `json:"isolation_mark"`
}

  func (s *Service) Observe(ctx context.Context) (ObserveSnap, error) {
	  if s == nil {
		  return ObserveSnap{}, ErrUnavailable
	  }
	  gen, err := s.Generation(ctx)
	   if err != nil {
		  return ObserveSnap{}, err
	   }
	   open, err := s.store.ListOpenLeases(ctx)
	   if err != nil {
		   return ObserveSnap{}, err
	  }
	  proofs, err := s.store.ListOpenProofs(ctx)
	   if err != nil {
		  return ObserveSnap{}, err
	   }
	   return ObserveSnap{
		   Gen:   gen,
		   Stats: s.Stats(),
		   Open:  len(open),
		  Proofs: len(proofs),
		   Mark: IsolationMark,
	  }, nil
   }
