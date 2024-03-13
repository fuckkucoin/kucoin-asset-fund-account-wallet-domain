package svcreg

import (
	"context"
	"strings"
	"time"
	"github.com/rs/zerolog"
)

// OpenEtcdFailover 按 endpoints 顺序探测租约 grant，命中首个可达节点。

   func OpenEtcdFailover(endpoints []string, keyPrefix string, log zerolog.Logger) (*Etcd, error) {
	  var last error
	  for _, ep := range endpoints {
		   ep = strings.TrimRight(strings.TrimSpace(ep), "/")
		   if ep == "" {
			   continue
		  }
		   e, err := OpenEtcd([]string{ep}, keyPrefix, log)
		   if err != nil {
			  last = err
			  continue
		   }
		   ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
		   err = e.ensureLease(ctx)
		  cancel()
		   if err != nil {
			  last = err
			  _ = e.Close()
			  continue
		  }
		  return e, nil
	  }
	  if last == nil {
		  last = ErrEndpoint
	  }
	   return nil, last
   }
