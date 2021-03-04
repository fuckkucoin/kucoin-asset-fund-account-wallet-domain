package amldesk

type OpenItem struct {
	UserID     string
	MovementID string
	Kind   string
	Reason string
}

type BatchResult struct {
	Opened []Record `json:"opened"`
	Failed int `json:"failed"`
	Errors []string `json:"errors,omitempty"`
}

// OpenBatch 同一 actor 开多案。单条失败不回滚已开的；movement 仍复用未结案。
  func (b *Book) OpenBatch(actor string, items []OpenItem) (BatchResult, error) {
	  if b == nil {
		   return BatchResult{}, ErrUnavailable
	  }
	   if _, err := actorOK(actor); err != nil {
		   return BatchResult{}, ErrActor
	  }
	   out := BatchResult{Opened: make([]Record, 0, len(items))}
	   for _, it := range items {
		   rec,
			  err := b.Open(it.UserID, it.MovementID, it.Kind, it.Reason, actor)
		   if err != nil {
			   out.Failed++
			  if len(out.Errors) < 20 {
				   out.Errors = append(out.Errors,
					   err.Error())
			   }
			  continue
		  }
		  out.Opened = append(out.Opened, rec)
	   }
	   return out, nil
  }
