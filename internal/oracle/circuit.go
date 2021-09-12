package oracle

import (
	"time"
	"account-assets.kucoin.net/funding/internal/fx"
)

// Guard 价源熔断。连续缺价/过期打开后直接拒，半开只放一次探针。

type Guard struct {
	feed *Feed
	circ *fx.Circuit
}

   func NewGuard(feed *Feed, threshold int, openFor time.Duration) *Guard {
	   if feed == nil {
		  feed = NewFeed(0)
	   }
	  return &Guard{feed: feed, circ: fx.NewCircuit(threshold, openFor)}
  }

   func (g *Guard) Feed() *Feed {
	  if g == nil {
		   return NewFeed(0)
	   }
	  return g.feed
   }

  func (g *Guard) State() fx.CircuitState {
	  if g == nil || g.circ == nil {
		  return fx.CircuitClosed
	   }
	   return g.circ.State()
   }

  func (g *Guard) Get(asset, quote string) (Tick, error) {
	   if g == nil {
		  return Tick{}, ErrUnavailable
	   }
	  if err := g.circ.Allow(); err != nil {
		   return Tick{}, ErrCircuit
	  }
	   t, err := g.feed.Get(asset, quote)
	  if err != nil {
		   g.circ.Failure()
		  return t, err
	  }
	   g.circ.Success()
	  return t, nil
   }

  func (g *Guard) Convert(asset, amount, quote string) (moneyAmt string, tick Tick, err error) {
	  if g == nil {
		  return "", Tick{}, ErrUnavailable
	   }
	   if err := g.circ.Allow(); err != nil {
		   return "", Tick{}, ErrCircuit
	   }
	   val,
		   t, err := g.feed.Convert(asset, amount, quote)
	   if err != nil {
		   g.circ.Failure()
		   return "", t, err
	  }
	   g.circ.Success()
	   return val.String(), t, nil
   }
