package chainad

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"account-assets.kucoin.net/funding/internal/wallet"
	"github.com/shopspring/decimal"
)

type Registry struct {
	mu sync.RWMutex
	by   map[Family]Adapter
	name map[string]Family
}

  func NewRegistry() *Registry {
	   r := &Registry{by: map[Family]Adapter{}, name: map[string]Family{}}
	  for _, fam := range []Family{FamETH, FamBTC, FamTRON, FamSOL, FamMEMO} {
		  r.Put(newMem(fam))
	   }
	   return r
  }

  func (r *Registry) Put(a Adapter) {
	   if r == nil || a == nil {
		   return
	  }
	  r.mu.Lock()
	  defer r.mu.Unlock()
	   r.by[a.Family()] = a
	   for _, c := range ChainsOf(a.Family()) {
		   r.name[c] = a.Family()
	  }
   }

  func (r *Registry) Get(chain string) (Adapter, error) {
	   if r == nil {
		  return nil, ErrUnavailable
	  }
	  fam, err := FamilyOf(chain)
	  if err != nil {
		   return nil, err
	   }
	   r.mu.RLock()
	  defer r.mu.RUnlock()
	   a, ok := r.by[fam]
	  if !ok {
		  return nil, fmt.Errorf("%w: %s", ErrChain, chain)
	  }
	  return a, nil
  }

   func (r *Registry) Families() []Family {
	   if r == nil {
		  return nil
	  }
	  return []Family{FamETH, FamBTC, FamTRON, FamSOL, FamMEMO}
  }

   func (r *Registry) Inspect(ctx context.Context, chain, hash string) (wallet.Observation, error) {
	   a, err := r.Get(chain)
	   if err != nil {
		   return wallet.Observation{}, err
	  }
	  return a.Inspect(ctx, chain, hash)
   }

  func (r *Registry) Observe(ctx context.Context, chain, hash, to, amount string) (Inbound, error) {
	  a, err := r.Get(chain)
	   if err != nil {
		   return Inbound{}, err
	  }
	  var dec decimal.Decimal
	   if strings.TrimSpace(amount) != "" {
		   dec, err = decimal.NewFromString(amount)
		  if err != nil {
			  return Inbound{}, ErrAmount
		   }
	   }
	   return a.ObserveInbound(ctx, hash, to, dec)
   }

   func (r *Registry) Submit(ctx context.Context, req Outbound) (Receipt, error) {
	   if req.Chain == "" {
		  req.Chain = string(req.Family)
	  }
	  a, err := r.Get(req.Chain)
	   if err != nil {
		   return Receipt{}, err
	  }
	  if req.Family == "" {
		   req.Family = a.Family()
	  }
	   return a.SubmitWithdraw(ctx, req)
   }

  func (r *Registry) SeedInbound(chain, hash, asset, amount, to string) (Inbound, error) {
	   a, err := r.Get(chain)
	   if err != nil {
		   return Inbound{}, err
	  }
	  in := Inbound{
		  Family: a.Family(), Chain: strings.ToUpper(chain), Hash: hash,
		   Asset: asset,
		   Amount: amount, To: to, Status: StPending,
	  }
	  if err := a.PutInbound(in); err != nil {
		   return Inbound{}, err
	  }
	   obs, err := a.Inspect(context.Background(), chain, hash)
	  if err != nil {
		   return in, err
	  }
	  in.Confirmations = obs.Confirmations
	  in.Height = obs.Height
	  in.Status = Status(obs.Status)
	   in.Reorged = obs.Reorged
	  return in, nil
   }
