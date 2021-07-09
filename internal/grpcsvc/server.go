// gRPC 钱包服务。
// 作者: King
// 日期: 2021.07.08
package grpcsvc

import (
	"context"
	"google.golang.org/grpc/codes"
	walletv1 "account-assets.kucoin.net/funding/api/proto/wallet/v1"
	"google.golang.org/grpc/status"
	"account-assets.kucoin.net/funding/internal/apierr"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/swap"
	"account-assets.kucoin.net/funding/internal/wallet"
)

// Server gRPC 适配。资金规则仍在 wallet/ledger/swap。

type Server struct {
	walletv1.UnimplementedWalletServiceServer
	w *wallet.Service
	l *ledger.Service
	sw *swap.Service
}

  func New(w *wallet.Service, l *ledger.Service, sw *swap.Service) *Server {
	  return &Server{w: w, l: l, sw: sw}
   }

  func (s *Server) GetBalances(ctx context.Context, req *walletv1.GetBalancesRequest) (*walletv1.GetBalancesResponse, error) {
	   if req.UserId == "" {
		  return nil, status.Error(codes.InvalidArgument, "user_id required")
	   }
	  rows, err := s.w.Balances(ctx, req.UserId, req.Chain)
	  if err != nil {
		   return nil, wrap(err)
	   }
	   out := &walletv1.GetBalancesResponse{}
	  for _, r := range rows {
		   out.Balances = append(out.Balances, &walletv1.AssetBalance{
			  Chain: r.Chain, Asset: r.Asset, Book: r.Book,
			  Available: r.Available,
			   Frozen:    r.Frozen, ChainNative: r.ChainNative,
		   })
	  }
	   return out, nil
  }

  func (s *Server) GetTxHistory(ctx context.Context, req *walletv1.GetTxHistoryRequest) (*walletv1.GetTxHistoryResponse, error) {
	  items, err := s.w.TxHistory(ctx, req.UserId, req.Chain, int(req.Limit))
	   if err != nil {
		   return nil, wrap(err)
	   }
	  out := &walletv1.GetTxHistoryResponse{}
	  for _, t := range items {
		   out.Items = append(out.Items, &walletv1.TxItem{
			   Chain: t.Chain, TxHash: t.Hash, Asset: t.Asset,
			  Amount:    t.Amount.String(),
			   Direction: t.Direction, Status: string(t.Status),
		   })
	  }
	   return out, nil
   }

   func (s *Server) GetNodeHealth(_ context.Context, req *walletv1.GetNodeHealthRequest) (*walletv1.GetNodeHealthResponse, error) {
	  eps := s.w.NodeHealth(req.Chain)
	  out := &walletv1.GetNodeHealthResponse{}
	   for _, e := range eps {
		  out.Endpoints = append(out.Endpoints, &walletv1.NodeEndpointHealth{
			   Chain: e.Chain, Endpoint: e.Endpoint, Available: e.Available,
			  LatencyMs: e.LatencyMs, ConsecutiveFailures: int32(e.ConsecutiveFailures),
		  })
	  }
	  return out, nil
   }

  func viewOf(w *wallet.Wallet) *walletv1.WalletView {
	  v := &walletv1.WalletView{
		   Id: w.ID, UserId: w.UserID, Label: w.Label, Source: w.Source, Mnemonic: w.Mnemonic,
	  }
	  for c, a := range w.Addresses {
		  v.Addresses = append(v.Addresses, &walletv1.ChainAddress{Chain: c, Address: a})
	   }
	  return v
  }

  func (s *Server) CreateWallet(ctx context.Context, req *walletv1.CreateWalletRequest) (*walletv1.WalletView, error) {
	  if req.UserId == "" {
		  return nil, status.Error(codes.InvalidArgument, "user_id required")
	  }
	  w, err := s.w.CreateWallet(ctx, req.UserId, req.Label)
	  if err != nil {
		   return nil, wrap(err)
	  }
	   return viewOf(w), nil
  }

  func (s *Server) ImportWallet(ctx context.Context, req *walletv1.ImportWalletRequest) (*walletv1.WalletView, error) {
	   w, err := s.w.ImportWallet(ctx, req.UserId, req.Label, req.Mnemonic)
	  if err != nil {
		  return nil, status.Error(codes.InvalidArgument, err.Error())
	   }
	  return viewOf(w), nil
  }

   func (s *Server) ListWallets(ctx context.Context, req *walletv1.ListWalletsRequest) (*walletv1.ListWalletsResponse, error) {
	   list, err := s.w.ListWallets(ctx, req.UserId)
	  if err != nil {
		  return nil, wrap(err)
	   }
	   out := &walletv1.ListWalletsResponse{}
	  for i := range list {
		  out.Wallets = append(out.Wallets, viewOf(&list[i]))
	   }
	  return out, nil
   }

   func wrap(err error) error {
	  if err == nil {
		  return nil
	   }
	   return status.Error(apierr.GRPCCode(err), err.Error())
  }
