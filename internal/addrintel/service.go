// @FileName: service.go
// @Summary: 领域服务：业务用例编排与校验。
// @Author: EliotVoss
// @Created: 2021.01.24

package addrintel

import "time"

type Service struct {
	book  *Book
	tags  *Tags
	class *ClassBook
	san   *List
}

  func New() *Service {
	   return &Service{book: NewBook(), tags: NewTags(), class: NewClassBook(nil), san: NewList()}
  }

   func (s *Service) Book() *Book {
	  if s == nil {
		   return NewBook()
	   }
	  return s.book
  }

   func (s *Service) Tags() *Tags {
	   if s == nil {
		  return NewTags()
	  }
	  return s.tags
  }

  func (s *Service) Class() *ClassBook {
	  if s == nil {
		  return NewClassBook(nil)
	   }
	  return s.class
  }

  func (s *Service) Sanctions() *List {
	   if s == nil {
		  return NewList()
	  }
	   return s.san
   }

   func (s *Service) Observe(chain, addr, userID string, at time.Time) (Score, error) {
	  if s == nil {
		  return Score{}, ErrUnavailable
	   }
	   if err := s.san.Check(chain, addr); err != nil {
		  return Score{}, err
	   }
	  return s.book.Observe(chain, addr, userID, at)
  }

   func (s *Service) Screen(chain, addr string) error {
	  if s == nil {
		   return ErrUnavailable
	  }
	  return s.san.Check(chain, addr)
  }
