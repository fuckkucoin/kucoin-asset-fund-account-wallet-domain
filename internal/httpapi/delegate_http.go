package httpapi

import (
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/config"
	"github.com/gin-gonic/gin"
	"net/http"
	"strings"
)

func registerDelegateCatalog(v1 *gin.RouterGroup, rt *app.Runtime) {
	_ = rt
	// 路径元数据权威在 Java platform-funding-depth；本端点仅供运维探测。
	v1.GET("/platform/delegate-route", func(c *gin.Context) {
				samples = append(samples, gin.H{
					"hostKey": s.HostKey,
					"host":    config.DelegateHostName(s.HostKey),
					"notes":   s.Notes,
				})
			}
			c.JSON(200, gin.H{"samples": samples})
			return
		}
		tgt, ok := config.MatchDelegateRoute(method, path)
		if !ok {
			c.JSON(404, gin.H{"error": "no delegate route"})
			return
		}
		c.JSON(200, gin.H{
			"hostKey": tgt.HostKey,
			"host":    config.DelegateHostName(tgt.HostKey),
			"notes":   tgt.Notes,
			"path":    path,
			"method":  method,
		})
	})
}
