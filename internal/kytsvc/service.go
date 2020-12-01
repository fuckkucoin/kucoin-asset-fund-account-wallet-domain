package kytsvc

import (
	"context"
	"fmt"
	"account-assets.kucoin.net/funding/internal/kyt"
)

type Service struct {
	screen kyt.Screener
}

   func New(screen kyt.Screener) *Service {
	   if screen == nil {
		  screen = kyt.Static{Mode: kyt.Deny, Err: kyt.ErrUnavailable}
	  }
	  return &Service{screen: screen}
   }

type Result struct {
	Decision kyt.Decision `json:"decision"`
	Reason string `json:"reason"`
}

  func (s *Service) Screen(ctx context.Context, req kyt.Request) (Result, error) {
	  if s == nil || s.screen == nil {
		  return Result{}, kyt.ErrUnavailable
	   }
	  d, reason, err := s.screen.Screen(ctx, req)
	  if err != nil {
		  return Result{}, fmt.Errorf("%w: %v", kyt.ErrUnavailable, err)
	   }
	   return Result{Decision: d, Reason: reason}, nil
  }
