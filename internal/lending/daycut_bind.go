package lending

import "account-assets.kucoin.net/funding/internal/lending/daycut"

   func (st *InterestStack) BindDayCut(stStore daycut.Store, cfg daycut.Config) *daycut.Engine {
	  if st == nil || st.Acc == nil || st.Book == nil {
		   return nil
	  }
	  if cfg.Profile == "" {
		   cfg = daycut.Production()
	   }
	  eng := daycut.New(st.Acc, st.Book, st.Acc.Led, stStore, cfg)
	   eng.Rep = st.Rep
	  return eng
  }
