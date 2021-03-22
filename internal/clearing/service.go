package clearing

import "time"

type Service struct {
	book *Book
}

  func New() *Service { return &Service{book: NewBook()} }

  func (s *Service) Book() *Book {
	  if s == nil {
		  return NewBook()
	  }
	  if s.book == nil {
		   s.book = NewBook()
	   }
	   return s.book
   }

  func (s *Service) Open(day, actor string) (Batch, error) {
	  return s.Book().OpenBatch(day, actor)
   }

   func (s *Service) Enqueue(batchID, userID, asset, amount, movement string, offset int, trade time.Time) (Item, error) {
	   return s.Book().Add(batchID, userID, asset, amount, movement, offset, trade, s.Book().skipWE)
   }

  func (s *Service) Tick(now time.Time) []Item {
	   return s.Book().ScanReady(now)
   }

   func (s *Service) Release(id, actor string, epoch int64, token string) (Item, error) {
	  return s.Book().Release(id, actor, epoch, token)
  }

  func (s *Service) Close(id, actor string, epoch int64, token string) (Batch, error) {
	   return s.Book().CloseBatch(id,
		  actor, epoch, token)
   }

  func (s *Service) Park(itemID, actor, reason string) (Exception, error) {
	   return s.Book().Park(itemID, actor, reason)
  }
