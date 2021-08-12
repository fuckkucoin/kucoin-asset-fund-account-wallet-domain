package ledorphan

import (
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/persist"
	"github.com/gin-gonic/gin"
)

// Register 将孤儿封堵引擎的全部 HTTP 路由注册到 v1 路由组。
// 若 rt 为 nil（内存模式），以无 KV 的只读模式启动。
  func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	  svc := New()
	   if rt != nil && rt.KV != nil {
		  svc.BindKV(rt.KV)
	  }
	  h := NewHandlers(svc)
	   // 报告类端点（只读）。
	   v1.GET("/ledorphan/reports", h.ListReports)
	   v1.GET("/ledorphan/reports/:report_id", h.GetReport)
	  // 扫描触发（ops.admin）。
	   v1.POST("/ledorphan/scan", h.TriggerScan)
	   // 封堵动作（ops.admin + 双人授权）。
	  v1.POST("/ledorphan/seal", h.SealOrphans)
	   v1.GET("/ledorphan/seal/:seal_id", h.GetSeal)
	   // 完整性证明。
	   v1.GET("/ledorphan/proof/:batch_id", h.ExportProof)
	   v1.GET("/ledorphan/proof/id/:proof_id", h.GetProof)
	  v1.POST("/ledorphan/proof/verify", h.VerifyProof)
	  // 世代锁管理（内部 worker 接口）。
	   v1.GET("/ledorphan/gen/:batch_id", h.GetGen)
	  v1.POST("/ledorphan/gen/acquire", h.AcquireGen)
	  v1.POST("/ledorphan/gen/release", h.ReleaseGen)
   }

// BindToService 将 ledbatch 和 ledger 适配器注入 Service 的工厂方法。
// 供 cmd/led-orphan 和 app.Runtime 集成使用。
  func BindToService(svc *Service, kv persist.KV, batchAdapter BatchObserver, ledgerAdapter LedgerObserver, reverseHop ReverseFunc) *Service {
	   if svc == nil {
		  svc = New()
	  }
	  if kv != nil {
		  svc.BindKV(kv)
	  }
	  if batchAdapter != nil {
		   svc.BindBatchObserver(batchAdapter)
	   }
	  if ledgerAdapter != nil {
		  svc.BindLedgerObserver(ledgerAdapter)
	  }
	  if reverseHop != nil {
		  svc.BindReverseHop(reverseHop)
	  }
	   return svc
   }
