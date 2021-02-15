package addrintel

import "time"

type ScreenRow struct {
	Chain   string `json:"chain"`
	Address string `json:"address"`
	OK bool `json:"ok"`
	Kind Kind   `json:"kind,omitempty"`
	Err  string `json:"error,omitempty"`
}

// ScreenBatch 逐条过进程内名单和分类。任一条制裁命中整批仍返回行结果，调用方自己决定是否关闸。
   func (s *Service) ScreenBatch(rows []struct{ Chain, Address string }) []ScreenRow {
	   out := make([]ScreenRow, 0, len(rows))
	  for _, r := range rows {
		  item := ScreenRow{Chain: r.Chain, Address: r.Address, OK: true}
		  if err := s.Screen(r.Chain, r.Address); err != nil {
			   item.OK = false
			   item.Err = err.Error()
			   out = append(out, item)
			  continue
		  }
		   k, err := s.Class().KindOf(r.Chain, r.Address)
		   if err != nil {
			  item.OK = false
			  item.Err = err.Error()
		  } else {
			  item.Kind = k
		   }
		  out = append(out, item)
	  }
	   return out
   }

   func (s *Service) ObserveBatch(hits []Hit) ([]Score, error) {
	   out := make([]Score,
		   0, len(hits))
	   for _, h := range hits {
		  at := h.At
		   if at.IsZero() {
			  at = time.Now().UTC()
		  }
		  sc, err := s.Observe(h.Chain, h.Address, h.UserID, at)
		  if err != nil {
			   return out, err
		  }
		  out = append(out, sc)
	   }
	  return out, nil
   }
