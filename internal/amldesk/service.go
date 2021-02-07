package amldesk

type Service struct {
	book *Book
	watch *Watchlist
}

  func New() *Service { return &Service{book: NewBook(), watch: NewWatchlist()} }

   func (s *Service) Book() *Book {
	  if s == nil {
		   return nil
	  }
	  return s.book
   }

   func (s *Service) Watch() *Watchlist {
	  if s == nil {
		  return NewWatchlist()
	   }
	  if s.watch == nil {
		   s.watch = NewWatchlist()
	   }
	  return s.watch
   }

   func (s *Service) Open(userID, movementID, kind, reason, actor string) (Record, error) {
	  if s == nil || s.book == nil {
		   return Record{}, ErrUnavailable
	  }
	  return s.book.Open(userID, movementID, kind, reason, actor)
  }

  func (s *Service) Attach(caseID, actor, kind, body, note string) (Evidence, error) {
	   if s == nil || s.book == nil {
		   return Evidence{}, ErrUnavailable
	   }
	  return s.book.Attach(caseID, actor, kind, body, note)
  }

  func (s *Service) Close(id, actor, decision string, epoch int64, token, note string) (Record, error) {
	   if s == nil || s.book == nil {
		   return Record{}, ErrUnavailable
	   }
	   return s.book.CloseFourEye(id, actor, decision, epoch, token, note)
   }

  func (s *Service) Get(id string) (Record, error) {
	   if s == nil || s.book == nil {
		   return Record{}, ErrUnavailable
	  }
	  return s.book.Get(id)
   }

  func (s *Service) Queue() []Record {
	  if s == nil || s.book == nil {
		  return nil
	   }
	  return s.book.OpenQueue(50)
   }

   func (s *Service) Assign(id, actor, reviewer string, epoch int64, token string) (Record, error) {
	   if s == nil || s.book == nil {
		   return Record{},
			  ErrUnavailable
	  }
	  return s.book.Assign(id, actor, reviewer, epoch, token)
  }

  func (s *Service) OpenBatch(actor string, items []OpenItem) (BatchResult, error) {
	  if s == nil || s.book == nil {
		   return BatchResult{},
			  ErrUnavailable
	  }
	  return s.book.OpenBatch(actor, items)
  }
