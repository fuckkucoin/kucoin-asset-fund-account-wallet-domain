package brokerbook

import "account-assets.kucoin.net/funding/pkg/money"

   func (s *Service) TotalOf(masterID, asset string) (string, error) {
	  if s == nil {
		  return "", ErrUnavailable
	  }
	  if _, err := s.GetMaster(masterID); err != nil {
		   return "", err
	   }
	   mb,
		   err := s.BookOf(masterID, asset)
	  if err != nil {
		   return "", err
	   }
	  sum, err := money.ParseBook(orZ(mb.Available))
	  if err != nil {
		  return "", err
	  }
	  for _, sub := range s.ListSubs(masterID) {
		   sb, err := s.BookOf(sub.ID, asset)
		  if err != nil {
			   return "", err
		  }
		   got, err := money.ParseBook(orZ(sb.Available))
		   if err != nil {
			  return "", err
		   }
		   sum, err = sum.Add(got)
		  if err != nil {
			   return "", err
		  }
	  }
	   return sum.String(), nil
   }
