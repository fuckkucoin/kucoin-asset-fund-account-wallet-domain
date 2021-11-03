// @FileName: service.go
// @Summary: 链重组资金影响会计——领域服务装配。
// Service 聚合 Store / Ingestor / InventoryScanner，供上层 Worker 与 HTTP 统一调用。
// @Author: Likeo
// @Created: 2021.10.21

package reorgacct

import (
	"context"
	"database/sql"
)

// Service 链重组影响会计领域服务。
type Service struct {
	store    *Store
	ingestor *Ingestor
	scanner  *InventoryScanner
}

// NewService 构造 Service；db 不得为 nil，lookup 接口可为 nil（对应维度盘点跳过）。

   func NewService(
	  db *sql.DB,
	  deposit DepositFaceLookup,
	   occupancy OccupancyLookup,
	   hop HopLookup,
   ) *Service {
	  store := NewStore(db)
	  ingestor := NewIngestor(store)
	   scanner := NewInventoryScanner(store, ingestor, deposit, occupancy, hop)
	  return &Service{
		   store:    store,
		  ingestor: ingestor,
		  scanner:  scanner,
	   }
  }

// Ingest 摄取重组事件（委托给 Ingestor）。

   func (s *Service) Ingest(ctx context.Context, req IngestReq) (*ReorgEvent, error) {
	  return s.ingestor.Ingest(ctx, req)
  }

// ScanEvent 触发影响面盘点（委托给 InventoryScanner）。
  func (s *Service) ScanEvent(ctx context.Context, eventID string) ([]ImpactRow, error) {
	  ev, err := s.store.GetEvent(ctx, eventID)
	  if err != nil {
		  return nil, err
	  }
	  return s.scanner.Scan(ctx, ev)
  }

// GetImpactSummary 查询已盘点的影响面汇总。

  func (s *Service) GetImpactSummary(ctx context.Context, eventID string) (ImpactSummary, error) {
	   rows, err := s.store.ListImpactRows(ctx, eventID)
	   if err != nil {
		   return ImpactSummary{}, err
	  }
	  return Summarize(eventID, rows), nil
  }

// Close 人工关闭事件（推进到 closed 状态）。
   func (s *Service) Close(ctx context.Context, eventID string) error {
	   return s.ingestor.Transition(ctx, eventID, EventStateClosed)
   }

// Store 返回底层 Store（供 Worker / Handler 直接使用只读接口）。
   func (s *Service) Store() *Store {
	  return s.store
   }

// IngestorRef 返回底层 Ingestor。
   func (s *Service) IngestorRef() *Ingestor {
	  return s.ingestor
   }

// Scanner 返回底层 InventoryScanner。

  func (s *Service) Scanner() *InventoryScanner {
	  return s.scanner
   }
