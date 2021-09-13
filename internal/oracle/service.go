// 功能：领域服务：业务用例编排与校验。
// 作者：Bronson
// 日期：2021.09.12

package oracle

import "time"

type Service struct {
	feed *Feed
}

  func New(maxAge time.Duration) *Service {
	   return &Service{feed: NewFeed(maxAge)}
   }

   func NewMock(now time.Time) *Service {
	  return &Service{feed: Mock(now)}
   }

   func (s *Service) Feed() *Feed {
	  if s == nil {
		   return NewFeed(0)
	  }
	  if s.feed == nil {
		   s.feed = NewFeed(0)
	  }
	  return s.feed
  }

   func (s *Service) Put(asset, quote, price string, at time.Time) (Tick, error) {
	   t, err := ParseTick(asset, quote, price, at)
	   if err != nil {
		   return Tick{}, err
	  }
	   if err := s.Feed().Put(t); err != nil {
		   return Tick{}, err
	  }
	   return t, nil
  }

  func (s *Service) Get(asset, quote string) (Tick, error) {
	  return s.Feed().Get(asset, quote)
  }
