package sweeprun

import (
	"strings"
	"account-assets.kucoin.net/funding/pkg/money"
	"account-assets.kucoin.net/funding/internal/validate"
)

  func budgetKey(chain, asset string) string {
	  return strings.ToUpper(strings.TrimSpace(chain)) + "|" + strings.ToUpper(strings.TrimSpace(asset))
  }

   func (s *Service) day() string {
	   return s.now().UTC().Format("2006-01-02")
  }

  func (s *Service) SetBudget(chain, asset, cap string) error {
	  if s == nil {
		  return ErrUnavailable
	  }
	  if chain != "" {
		  if _, err := validate.Chain(chain); err != nil {
			   return ErrChain
		   }
	   }
	   if asset != "" {
		  if _, err := validate.Asset(asset); err != nil {
			  return ErrAsset
		   }
	   }
	   amt, err := money.Parse(cap)
	   if err != nil {
		   return ErrAmount
	  }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   key := budgetKey(chain, asset)
	  cur := s.budget[key]
	  if cur == nil {
		  cur = &Budget{Chain: strings.ToUpper(strings.TrimSpace(chain)),
			  Asset: strings.ToUpper(strings.TrimSpace(asset))}
		   s.budget[key] = cur
	  }
	  s.rollLocked(cur)
	   cur.DailyCap = amt.String()
	   return nil
   }

  func (s *Service) BudgetOf(chain, asset string) (Budget, error) {
	   if s == nil {
		  return Budget{}, ErrUnavailable
	   }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	   cur := s.budget[budgetKey(chain, asset)]
	  if cur == nil {
		   return Budget{}, ErrBudget
	  }
	   s.rollLocked(cur)
	  return cur.Copy(), nil
   }

   func (s *Service) RemainingBudget(chain, asset string) (string, error) {
	  b, err := s.BudgetOf(chain, asset)
	   if err != nil {
		  return "", err
	   }
	   cap, err := money.Parse(b.DailyCap)
	   if err != nil {
		  return "", ErrAmount
	  }
	   used,
		  err := money.ParseBook(orZ(b.Used))
	   if err != nil {
		  return "", ErrAmount
	  }
	   left, err := cap.Sub(used)
	   if err != nil {
		   return "", err
	   }
	   return left.String(), nil
  }

   func (s *Service) rollLocked(b *Budget) {
	  day := s.day()
	   if b.Day != day {
		  b.Day = day
		   b.Used = "0"
	   }
  }

   func (s *Service) keysFor(chain, asset string) []string {
	   return []string{
		   budgetKey("", ""),
		  budgetKey(chain, ""),
		   budgetKey("", asset),
		  budgetKey(chain, asset),
	  }
   }

   func (s *Service) checkBudgetLocked(chain, asset string, add money.Amount) error {
	  for _, key := range s.keysFor(chain, asset) {
		  b := s.budget[key]
		  if b == nil || strings.TrimSpace(b.DailyCap) == "" {
			   continue
		  }
		  s.rollLocked(b)
		   cap, err := money.Parse(b.DailyCap)
		  if err != nil {
			   return ErrAmount
		   }
		  used,
			  err := money.ParseBook(orZ(b.Used))
		  if err != nil {
			   return ErrAmount
		  }
		   next, err := used.Add(add)
		  if err != nil {
			   return err
		  }
		   if next.Cmp(cap) > 0 {
			  return ErrBudget
		   }
	  }
	   return nil
  }

  func (s *Service) addUsedLocked(chain, asset string, add money.Amount) error {
	  for _, key := range s.keysFor(chain, asset) {
		   b := s.budget[key]
		   if b == nil || strings.TrimSpace(b.DailyCap) == "" {
			  continue
		   }
		  s.rollLocked(b)
		   used, err := money.ParseBook(orZ(b.Used))
		  if err != nil {
			   return err
		   }
		  next, err := used.Add(add)
		   if err != nil {
			  return err
		  }
		   b.Used = next.String()
	   }
	  return nil
  }

   func (s *Service) subUsedLocked(chain, asset string, add money.Amount) error {
	   for _, key := range s.keysFor(chain, asset) {
		   b := s.budget[key]
		   if b == nil || strings.TrimSpace(b.DailyCap) == "" {
			   continue
		  }
		  used,
			  err := money.ParseBook(orZ(b.Used))
		   if err != nil {
			   return err
		   }
		   next, err := used.Sub(add)
		  if err != nil {
			   return err
		  }
		   b.Used = next.String()
	  }
	  return nil
   }

  func orZ(s string) string {
	  if strings.TrimSpace(s) == "" {
		  return "0"
	  }
	   return s
   }
