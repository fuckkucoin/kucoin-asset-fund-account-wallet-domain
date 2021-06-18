// @FileName: service.go
// @Summary: fundread 包内 service 实现。
// 强一致与最终一致读。强一致失败时仍带回当前水位，方便调用方看到差多少。
// @Author: Cason
// @Created: 2021.06.17

package fundread

import (
	"context"
	"database/sql"
	"fmt"
	"time"
	"account-assets.kucoin.net/funding/internal/fundmat"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/persist"
	"github.com/google/uuid"
)

// Service 用户资金视图读模型。
type Service struct {
	Mat *fundmat.Service
}

// New 绑定物化服务。
  func New(mat *fundmat.Service) *Service {
	   if mat == nil {
		   mat = fundmat.New(nil)
	   }
	  return &Service{Mat: mat}
   }

// NewWired 按库或内存装配，并挂上账本只读口。db 为空时用内存投影。

   func NewWired(db *sql.DB, dialect persist.Dialect, led *ledger.Service) *Service {
	   var st fundmat.Store
	   if db != nil {
		   st = fundmat.NewSQLStore(db, dialect)
	   } else {
		   st = fundmat.NewMem()
	  }
	   mat := fundmat.New(st)
	   if led != nil {
		   mat.BindLedger(led)
	  }
	   return New(mat)
   }

// Strong 强一致读。水位未覆盖围栏时返回 ErrNotCaughtUp，视图里仍带有水位。
  func (s *Service) Strong(ctx context.Context, q Query) (View, error) {
	   view, fence, err := s.prepare(ctx, q, "strong")
	  if err != nil {
		  return view, err
	  }
	   if err := RequireCaughtUp(view.Watermarks, fence, time.Now().UTC()); err != nil {
		  fundmat.NoteStrongRefuse()
		   view.Stale = true
		   view.Fence = fence
		   _ = s.Mat.PutFence(ctx, fence)
		   return view, err
	  }
	   view.Stale = false
	   view.Fence = fence
	  if err := s.Mat.PutFence(ctx, fence); err != nil {
		   return view, err
	  }
	   return view, nil
   }

// Eventual 最终一致读。允许返回尚未追平的投影，并标明 stale 与水位。
  func (s *Service) Eventual(ctx context.Context, q Query) (View, error) {
	  view, fence, err := s.prepare(ctx, q, "eventual")
	  if err != nil {
		  return View{}, err
	  }
	   view.Stale = behind(view.Watermarks)
	   view.Fence = fence
	  return view, nil
   }

// IssueFence 签发一条读围栏。minSeq 为 0 时取该流日志头；未指定流时不预填跨流序号。

  func (s *Service) IssueFence(ctx context.Context, q Query, ttl time.Duration) (fundmat.ReadFence, error) {
	   if s == nil || s.Mat == nil {
		  return fundmat.ReadFence{}, fundmat.ErrUnavailable
	   }
	  if q.UserID == "" {
		  return fundmat.ReadFence{}, ErrUser
	  }
	  rep, err := s.Mat.Lag(ctx)
	   if err != nil {
		  return fundmat.ReadFence{}, err
	   }
	  if ttl <= 0 {
		   ttl = 30 * time.Second
	   }
	   now := time.Now().UTC()
	  f := fundmat.ReadFence{
		  ID: uuid.NewString(), UserID: q.UserID, Stream: q.Stream,
		  Epoch: q.FenceEpoch,
		   MinSeq: q.FenceSeq, IssuedAt: now, ExpiresAt: now.Add(ttl),
	   }
	   if f.Stream != "" {
		  if wm, ok := findWM(rep.Streams, f.Stream); ok {
			  if f.Epoch == 0 {
				  f.Epoch = wm.Epoch
			   }
			   if f.MinSeq == 0 {
				   f.MinSeq = wm.HeadSeq
			  }
		   }
	   }
	   f.Token = fundmat.DigestToken(f.UserID,
		  f.Stream, f.Epoch, f.MinSeq, f.IssuedAt.Format(time.RFC3339Nano))
	   if err := s.Mat.PutFence(ctx, f); err != nil {
		  return fundmat.ReadFence{}, err
	  }
	   return f, nil
   }

  func (s *Service) prepare(ctx context.Context, q Query, consistency string) (View, fundmat.ReadFence, error) {
	   if s == nil || s.Mat == nil {
		  return View{}, fundmat.ReadFence{}, fundmat.ErrUnavailable
	  }
	  if q.UserID == "" {
		  return View{}, fundmat.ReadFence{}, ErrUser
	   }
	  rep, err := s.Mat.Lag(ctx)
	  if err != nil {
		   return View{}, fundmat.ReadFence{}, err
	   }
	   rows, err := s.Mat.Rows(ctx, q.UserID, q.Asset, q.Subject)
	   if err != nil {
		   return View{}, fundmat.ReadFence{}, err
	  }
	   if rows == nil {
		  rows = []fundmat.MatRow{}
	  }
	  assets, err := Rollup(rows)
	   if err != nil {
		  return View{}, fundmat.ReadFence{}, err
	  }
	   ver, err := s.Mat.Version(ctx, q.UserID)
	   if err != nil && err != fundmat.ErrNotFound {
		   return View{}, fundmat.ReadFence{}, err
	   }
	   fence, err := s.loadFence(ctx, q, rep)
	  if err != nil {
		  return View{}, fundmat.ReadFence{}, err
	  }
	  view := View{
		   UserID: q.UserID, Consistency: consistency, Rows: rows, Assets: assets,
		  Watermarks: rep.Streams, Version: ver,
	  }
	  return view, fence, nil
  }

  func (s *Service) loadFence(ctx context.Context, q Query, rep fundmat.LagReport) (fundmat.ReadFence, error) {
	   now := time.Now().UTC()
	   if q.FenceID != "" {
		   f, err := s.Mat.Fence(ctx, q.FenceID)
		  if err != nil {
			   return fundmat.ReadFence{}, err
		   }
		  if f.UserID != "" && f.UserID != q.UserID {
			   return fundmat.ReadFence{}, fmt.Errorf("%w: fence user", fundmat.ErrBadEvent)
		  }
		  return f, nil
	   }
	  f := fundmat.ReadFence{
		   ID: uuid.NewString(), UserID: q.UserID, Stream: q.Stream,
		  Epoch: q.FenceEpoch, MinSeq: q.FenceSeq, IssuedAt: now, ExpiresAt: now.Add(30 * time.Second),
	   }
	   if f.Stream != "" {
		  if wm, ok := findWM(rep.Streams, f.Stream); ok && f.Epoch == 0 {
			  f.Epoch = wm.Epoch
		  }
	   }
	   f.Token = fundmat.DigestToken(f.UserID, f.Stream, f.Epoch, f.MinSeq, f.IssuedAt.Format(time.RFC3339Nano))
	  return f, nil
  }

   func behind(wms []fundmat.MatWatermark) bool {
	   for _, wm := range wms {
		   if wm.ContiguousSeq < wm.HeadSeq {
			  return true
		   }
	  }
	  return false
   }

// Run 交给物化服务循环。

   func (s *Service) Run(ctx context.Context, every time.Duration) {
	   if s == nil || s.Mat == nil {
		  return
	   }
	   s.Mat.Run(ctx, every)
   }
