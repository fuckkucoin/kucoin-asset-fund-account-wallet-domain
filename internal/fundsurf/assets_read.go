package fundsurf

import (
	"errors"
	"strconv"
	"strings"
	"account-assets.kucoin.net/funding/internal/callerid"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/fundread"
	"account-assets.kucoin.net/funding/internal/mw"
	"github.com/gin-gonic/gin"
)

// RegisterAssetsRead 挂载冻结 CSV/JSON、网络目录与地址选网提示；不含安全写路径。

  func RegisterAssetsRead(v1 *gin.RouterGroup, h HTTP) {
	   if v1 == nil {
		   return
	  }
	  surf := h.Surf
	   if surf == nil {
		   surf = New(nil, nil, nil)
	   }
	   v1.GET("/assets/frozen", func(c *gin.Context) {
		   uid, err := callerid.Require(c, h.ID)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		   if strings.TrimSpace(c.Query("source")) == "projection" {
			   strong := !strings.EqualFold(c.Query("consistency"), "eventual")
			  var fence int64
			  if raw := strings.TrimSpace(c.Query("fence_seq")); raw != "" {
				   fence,
					  _ = strconv.ParseInt(raw, 10, 64)
			   }
			   rows, meta, err := surf.ProjectedFrozen(c.Request.Context(), uid, strong, fence)
			   if err != nil {
				   if errors.Is(err, fundread.ErrNotCaughtUp) || errors.Is(err, fundread.ErrFenceExpired) {
					  c.JSON(409, gin.H{
						  "error": "投影未追平", "detail": err.Error(), "stale": true,
						  "watermarks": meta.Watermarks, "user_id": uid,
					  })
					  return
				  }
				  mw.WriteProblem(c, err)
				   return
			   }
			   if rows == nil {
				   rows = []FrozenRow{}
			  }
			  c.JSON(200, gin.H{
				  "user_id": uid, "items": rows, "source": "projection",
				   "stale": meta.Stale, "watermarks": meta.Watermarks, "version": meta.Version,
				  "host": config.HostFundSurface,
			  })
			  return
		   }
		  rows, err := surf.Frozen(c.Request.Context(), uid)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   if strings.EqualFold(strings.TrimSpace(c.Query("format")), "csv") {
			  var bld strings.Builder
			   bld.WriteString("asset,book,subject,frozen,reason,ref,category\n")
			   for _, r := range rows {
				  bld.WriteString(strings.Join([]string{
					  csvCell(r.Asset), csvCell(r.Book), csvCell(r.Subject), csvCell(r.Frozen),
					  csvCell(r.Reason), csvCell(r.Ref), csvCell(r.Category),
				  }, ","))
				   bld.WriteByte('\n')
			   }
			  c.Header("Content-Disposition", "attachment; filename=frozen_assets.csv")
			  c.Data(200, "text/csv; charset=utf-8", []byte(bld.String()))
			   return
		  }
		  c.JSON(200, gin.H{"user_id": uid, "items": rows, "host": config.HostFundSurface})
	  })
   }
