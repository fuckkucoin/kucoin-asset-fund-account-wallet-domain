// gRPC 划转和兑换。
// 作者: Span
// 日期: 2021.07.08
package grpcsvc

import (
	"context"
	walletv1 "account-assets.kucoin.net/funding/api/proto/wallet/v1"
	"account-assets.kucoin.net/funding/internal/ledger"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"account-assets.kucoin.net/funding/internal/swap"
)

   func (s *Server) CreateTransfer(ctx context.Context, req *walletv1.CreateTransferRequest) (*walletv1.TransferView, error) {
	   if s.l == nil {
		  return nil, status.Error(codes.Unavailable, "ledger not wired")
	  }
	   if req.UserId == "" || req.IdempotencyKey == "" {
		  return nil, status.Error(codes.InvalidArgument, "user_id and idempotency_key required")
	   }
	   amt, err := ledger.ParseAmount(req.Amount)
	   if err != nil {
		  return nil, wrap(err)
	   }
	   dir, err := ledger.ParseDirection(req.Direction)
	   if err != nil {
		   return nil, wrap(err)
	  }
	  tr, err := s.l.Create(ctx, ledger.CreateReq{
		  UserID: req.UserId, Direction: dir, Asset: req.Asset, Amount: amt,
		  Chain:     req.Chain,
		   ToAddress: req.ToAddress, TxHash: req.TxHash,
		  IdempotencyKey: req.IdempotencyKey,
		  WaitConfirm: req.Wait,
	  })
	  if err != nil {
		   return protoTransfer(tr),
			   wrap(err)
	  }
	   return protoTransfer(tr), nil
   }

  func (s *Server) GetTransfer(ctx context.Context, req *walletv1.GetTransferRequest) (*walletv1.TransferView, error) {
	  if s.l == nil {
		   return nil, status.Error(codes.Unavailable, "ledger not wired")
	   }
	  tr, err := s.l.Get(ctx, req.Id)
	   if err != nil {
		  return nil, wrap(err)
	  }
	   return protoTransfer(tr), nil
  }

   func (s *Server) ListTransfers(ctx context.Context, req *walletv1.ListTransfersRequest) (*walletv1.ListTransfersResponse, error) {
	  if s.l == nil {
		   return nil, status.Error(codes.Unavailable, "ledger not wired")
	  }
	  limit := int(req.Limit)
	  if limit <= 0 {
		  limit = 20
	  }
	   list, err := s.l.List(ctx, req.UserId, limit)
	   if err != nil {
		   return nil, wrap(err)
	   }
	   out := &walletv1.ListTransfersResponse{}
	  for _, tr := range list {
		   if !ledger.MatchTransfer(tr, req.Status, req.Direction, req.State) {
			   continue
		   }
		   out.Transfers = append(out.Transfers,
			   protoTransfer(tr))
	  }
	   return out, nil
   }

   func (s *Server) ListAccounts(ctx context.Context, req *walletv1.ListAccountsRequest) (*walletv1.ListAccountsResponse, error) {
	  if s.l == nil {
		  return nil, status.Error(codes.Unavailable, "ledger not wired")
	  }
	   books, err := s.l.Accounts(ctx, req.UserId)
	  if err != nil {
		   return nil, wrap(err)
	  }
	  out := &walletv1.ListAccountsResponse{UserId: req.UserId}
	   for _, b := range books {
		  out.Accounts = append(out.Accounts, &walletv1.AccountView{
			   Asset: b.Asset, Book: b.Book, Subject: b.Subject,
			   Available: b.Available,
			  Frozen: b.Frozen, Total: b.Total,
		  })
	  }
	   if s.sw != nil {
		  orders,
			   err := s.sw.ListOrders(ctx, req.UserId)
		   if err != nil {
			   return nil, wrap(err)
		  }
		  for _, f := range swap.FeesPaid(orders) {
			  out.FeesPaid = append(out.FeesPaid, &walletv1.FeePaidView{Asset: f.Asset, Amount: f.Amount, Subject: f.Subject})
		  }
	   }
	   return out, nil
   }

   func (s *Server) QuoteSwap(ctx context.Context, req *walletv1.QuoteSwapRequest) (*walletv1.QuoteView, error) {
	   if s.sw == nil {
		  return nil, status.Error(codes.Unavailable, "swap not wired")
	   }
	   q,
		   err := s.sw.Quote(ctx, req.FromAsset, req.ToAsset, req.Amount)
	   if err != nil {
		  return nil, wrap(err)
	  }
	  return &walletv1.QuoteView{
		   FromAsset: q.FromAsset, ToAsset: q.ToAsset, FromAmount: q.FromAmount,
		   Rate: q.Rate, ToAmount: q.ToAmount, FeeAsset: q.FeeAsset, FeeAmount: q.FeeAmount,
		   ExpiresAt: q.ExpiresAt, Source: q.Source,
	  }, nil
   }

  func (s *Server) ExecuteSwap(ctx context.Context, req *walletv1.ExecuteSwapRequest) (*walletv1.SwapOrderView, error) {
	  if s.sw == nil {
		  return nil, status.Error(codes.Unavailable, "swap not wired")
	  }
	  if req.IdempotencyKey == "" {
		  return nil, status.Error(codes.InvalidArgument, "idempotency_key required")
	  }
	   ord, err := s.sw.Execute(ctx, swap.ExecReq{
		   UserID: req.UserId, FromAsset: req.FromAsset, ToAsset: req.ToAsset,
		   FromAmount: req.Amount, IdempotencyKey: req.IdempotencyKey, FillRatio: req.FillRatio,
		  Chain: req.Chain, QuoteRate: req.QuoteRate, Async: !req.Wait,
	   })
	  if err != nil {
		  return protoSwap(ord), wrap(err)
	  }
	  return protoSwap(ord), nil
  }

   func (s *Server) GetSwapOrder(ctx context.Context, req *walletv1.GetSwapOrderRequest) (*walletv1.SwapOrderView, error) {
	   if s.sw == nil {
		   return nil, status.Error(codes.Unavailable, "swap not wired")
	  }
	   o, err := s.sw.Get(ctx, req.Id)
	   if err != nil {
		   return nil, wrap(err)
	   }
	   return protoSwap(o), nil
   }

  func (s *Server) ListSwapOrders(ctx context.Context, req *walletv1.ListSwapOrdersRequest) (*walletv1.ListSwapOrdersResponse, error) {
	  if s.sw == nil {
		   return nil, status.Error(codes.Unavailable, "swap not wired")
	   }
	  list, err := s.sw.ListOrders(ctx, req.UserId)
	  if err != nil {
		  return nil, wrap(err)
	  }
	  out := &walletv1.ListSwapOrdersResponse{}
	  for _, o := range list {
		   if !swap.MatchOrder(o, req.Status) {
			  continue
		   }
		  out.Orders = append(out.Orders, protoSwap(o))
	   }
	   return out, nil
  }

   func (s *Server) ListAddressBook(ctx context.Context, req *walletv1.ListAddressBookRequest) (*walletv1.ListAddressBookResponse, error) {
	   if s.l == nil {
		   return nil, status.Error(codes.Unavailable, "ledger not wired")
	   }
	   list, err := s.l.ListAddressBook(ctx, req.UserId)
	  if err != nil {
		   return nil, wrap(err)
	  }
	  out := &walletv1.ListAddressBookResponse{}
	  for _, e := range list {
		   out.Entries = append(out.Entries, protoAddr(e))
	   }
	   return out, nil
   }

   func (s *Server) AddAddressBook(ctx context.Context, req *walletv1.AddressBookEntry) (*walletv1.AddressBookEntry, error) {
	   if s.l == nil {
		   return nil, status.Error(codes.Unavailable, "ledger not wired")
	   }
	   got, err := s.l.AddAddressBook(ctx, ledger.AddressBookEntry{
		  UserID: req.UserId, Chain: req.Chain, Address: req.Address, Label: req.Label,
	  })
	   if err != nil {
		  return nil, wrap(err)
	  }
	  return protoAddr(got), nil
   }

  func (s *Server) RemoveAddressBook(ctx context.Context, req *walletv1.RemoveAddressBookRequest) (*walletv1.OkReply, error) {
	   if s.l == nil {
		  return nil, status.Error(codes.Unavailable, "ledger not wired")
	  }
	   if err := s.l.RemoveAddressBook(ctx, req.UserId, req.Chain, req.Address); err != nil {
		   return nil, wrap(err)
	   }
	  return &walletv1.OkReply{Ok: true}, nil
  }

   func protoTransfer(tr *ledger.Transfer) *walletv1.TransferView {
	   if tr == nil {
		   return nil
	   }
	   v := ledger.ViewTransfer(tr, "")
	   return &walletv1.TransferView{
		   Id:      v.ID,
		   OrderId: v.OrderID, ClientOid: v.ClientOid, MovementId: v.MovementID,
		   UserId: v.UserID, Direction: v.Direction, Settlement: v.Settlement,
		   Asset: v.Asset, Amount: v.Amount, FilledAmount: v.FilledAmount, RemainAmount: v.RemainAmount,
		  Chain: v.Chain, TxHash: v.TxHash, FromAccount: v.FromAccount, ToAccount: v.ToAccount,
		   Status: v.Status, StatusDesc: v.StatusDesc, State: v.State, FailReason: v.FailReason,
		  CreatedAtMs: v.CreatedAtMs,
	   }
   }

  func protoSwap(o *swap.Order) *walletv1.SwapOrderView {
	  if o == nil {
		  return nil
	   }
	   v := swap.ViewOrder(o)
	  return &walletv1.SwapOrderView{
		   Id: v.ID, OrderId: v.OrderID, ClientOid: v.ClientOid, MovementId: v.MovementID,
		   UserId: v.UserID, FromAsset: v.FromAsset, ToAsset: v.ToAsset,
		  FromAmount: v.FromAmount, FilledAmount: v.FilledAmount, RemainAmount: v.RemainAmount,
		   ToAmount: v.ToAmount, FeeAsset: v.FeeAsset, FeeAmount: v.FeeAmount, Account: v.Account,
		   Status:     v.Status,
		  StatusDesc: v.StatusDesc, State: v.State,
		   Chain: v.Chain, TxHash: v.TxHash, FailReason: v.FailReason, CreatedAtMs: v.CreatedAtMs,
	  }
   }

   func protoAddr(e ledger.AddressBookEntry) *walletv1.AddressBookEntry {
	   return &walletv1.AddressBookEntry{
		   UserId: e.UserID, Chain: e.Chain, Address: e.Address, Label: e.Label, Whitelist: e.Whitelist,
	  }
   }
