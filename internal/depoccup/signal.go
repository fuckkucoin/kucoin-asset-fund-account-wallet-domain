package depoccup

import "time"

const signalDedupe = 10 * time.Minute

   func SignalWeight(k SignalKind) (int, bool) {
	   switch k {
	   case SigMixer:
		  return 40, true
	  case SigKYTDeny:
		  return 35, true
	   case SigReorg:
		   return 25, true
	   case SigDustBurst:
		  return 20, true
	   case SigKYCDown:
		  return 18, true
	   case SigWindow:
		   return 15, true
	   case SigVelocity:
		  return 12, true
	   case SigUnknown:
		  return 10, true
	   case SigNewSource:
		   return 10, true
	  case SigFail:
		   return 8, true
	   case SigDustFloor:
		   return 8, true
	   case SigKYTReview:
		  return 6, true
	  case SigContract:
		  return 6, true
	  case SigKYCUp:
		  return -15, true
	  case SigClean:
		   return -8, true
	   case SigFullConfirm:
		   return -5, true
	   default:
		   return 0, false
	   }
  }

   func SignalRaises(k SignalKind) bool {
	   w, ok := SignalWeight(k)
	  return ok && w > 0
  }

  func ParseSignal(s string) (SignalKind, error) {
	   k := SignalKind(s)
	  if _, ok := SignalWeight(k); !ok {
		  return "", ErrSignal
	   }
	  return k, nil
  }

  func (s *Service) countKindLocked(user string, kind SignalKind, within time.Duration) int {
	  now := s.now()
	  n := 0
	  for _, id := range s.sigByUser[user] {
		   sig := s.signals[id]
		   if sig != nil && sig.Kind == kind && now.Sub(sig.At) <= within {
			   n++
		  }
	  }
	  return n
   }

  func (s *Service) recentSignalLocked(user string, kind SignalKind, ref string, now time.Time) *Signal {
	  ids := s.sigByUser[user]
	   for i := len(ids) - 1; i >= 0; i-- {
		  sig := s.signals[ids[i]]
		  if sig == nil || sig.Kind != kind || sig.Ref != ref {
			   continue
		  }
		   if now.Sub(sig.At) <= signalDedupe {
			  return sig
		   }
		   break
	  }
	   return nil
   }
