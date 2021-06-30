package fiatrail

import "context"

// RunToSettle 预约→KYC→授权→入账。任一步失败即停，不得半入账。
   func (s *Service) RunToSettle(ctx context.Context, req BookReq, actor string) (Booking, error) {
	   b, err := s.Book(req)
	   if err != nil {
		   return Booking{}, err
	  }
	  if got, err := s.ScreenKYC(b.ID); err != nil {
		   return got, err
	   }
	  if got, err := s.Authorize(b.ID); err != nil {
		   return got, err
	   }
	  return s.Settle(ctx, b.ID, actor)
  }
