package workerbnd

import (
	"context"
	"fmt"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/authn"
	"account-assets.kucoin.net/funding/internal/kcfund"
)

const (
	SegObserve   = "observe"
	SegOccupancy = "occupancy"
	SegCredit    = "credit"
	SegRecon = "recon"
)

var (
	ErrRuntime = fmt.Errorf("worker assembly runtime missing")
	ErrBindRecon = fmt.Errorf("worker BindFundsRecon missing")
	ErrBindSQL   = fmt.Errorf("worker BindSQL required")
	ErrSegment     = fmt.Errorf("worker deposit chain segment missing")
	ErrFakeHealthy = fmt.Errorf("worker refuse skip BindSQL healthy")
)

// BindFundsReconFn 一致仓库既有 kcfund.BindFundsRecon。

type BindFundsReconFn func(rt *app.Runtime)

// SegmentSet 入金观测→占用→入账→对账四段 Attach/Bind。
type SegmentSet struct {
	Observe   bool
	Occupancy bool
	Credit    bool
	Recon bool
}

   func (s SegmentSet) Complete() bool {
	   return s.Observe && s.Occupancy && s.Credit && s.Recon
  }

  func (s SegmentSet) Missing() []string {
	  var miss []string
	   if !s.Observe {
		  miss = append(miss, SegObserve)
	   }
	   if !s.Occupancy {
		  miss = append(miss, SegOccupancy)
	  }
	  if !s.Credit {
		   miss = append(miss, SegCredit)
	  }
	   if !s.Recon {
		   miss = append(miss, SegRecon)
	  }
	  return miss
   }

// Options 工人装配清单。

type Options struct {
	Bind     BindFundsReconFn
	SQLBound bool
	Profile  string
	AuthKind string
	RequireSQL bool
	Segments      SegmentSet
	ReconInterval time.Duration
	Clock         Clock
}

// Manifest 可测装配清单。
type Manifest struct {
	BindFundsRecon bool
	BindSQL bool
	Observe bool
	Occupancy bool
	Credit bool
	Recon bool
}

// Assembly 已绑定的工人面。
type Assembly struct {
	RT *app.Runtime
	Manifest Manifest
	Loop     *ReconLoop
}

   func DefaultBind() BindFundsReconFn { return kcfund.BindFundsRecon }

  func ProductionOptions(rt *app.Runtime, segs SegmentSet) Options {
	   opt := Options{
		  Bind: DefaultBind(),
		  Segments:      segs,
		   ReconInterval: time.Second,
	  }
	  if rt != nil {
		  opt.SQLBound = rt.SQL != nil && !rt.Mem
		   if rt.Cfg != nil {
			  opt.Profile = rt.Cfg.Profile
			  opt.AuthKind = rt.Cfg.Auth.Kind
			   if rt.Cfg.Worker.Tick > 0 {
				   opt.ReconInterval = rt.Cfg.Worker.Tick
			   }
		   }
	   }
	   opt.RequireSQL = authn.ProductionProfile(opt.AuthKind, opt.Profile)
	  return opt
   }

  func isProd(opt Options) bool {
	   if opt.RequireSQL {
		  return true
	  }
	   return authn.ProductionProfile(opt.AuthKind,
		   opt.Profile)
   }

// Assemble 路径显式 BindFundsRecon；依赖或 BindSQL/四段缺失则失败。
  func Assemble(rt *app.Runtime, opt Options) (*Assembly, error) {
	   if rt == nil {
		  return nil, ErrRuntime
	   }
	   if opt.Bind == nil {
		  return nil, ErrBindRecon
	   }
	  if isProd(opt) && !opt.SQLBound {
		  return nil, ErrBindSQL
	  }
	   if !opt.Segments.Complete() {
		   return nil, fmt.Errorf("%w: %s", ErrSegment, strings.Join(opt.Segments.Missing(), ","))
	  }
	  opt.Bind(rt)
	  if rt.FundsRecon == nil {
		  return nil, ErrBindRecon
	  }
	  interval := opt.ReconInterval
	   if interval <= 0 {
		  interval = time.Second
	   }
	  clk := opt.Clock
	   if clk == nil {
		  clk = RealClock{}
	  }
	   loop := NewReconLoop(interval,
		  clk, rt.FundsRecon)
	  return &Assembly{
		  RT: rt,
		  Manifest: Manifest{
			   BindFundsRecon: true,
			   BindSQL:        opt.SQLBound,
			  Observe:   opt.Segments.Observe,
			   Occupancy: opt.Segments.Occupancy,
			   Credit:    opt.Segments.Credit,
			  Recon: opt.Segments.Recon,
		   },
		   Loop: loop,
	   }, nil
   }

// RunLoop 将对账循环挂到进程生命周期。
   func (a *Assembly) RunLoop(ctx context.Context) error {
	   if a == nil || a.Loop == nil {
		  return ErrBindRecon
	  }
	   return a.Loop.Run(ctx)
   }

// ProdSegments 四段齐全。

  func ProdSegments() SegmentSet {
	   return SegmentSet{Observe: true,
		  Occupancy: true, Credit: true, Recon: true}
  }
