package integration

import (
	"context"
		"os"
	  "testing"
		"time" 
	"account-assets.kucoin.net/funding/internal/app"
		"account-assets.kucoin.net/funding/internal/platformjava"
) 

// TestJavaPlatformContractPing 在预发环境探测平台服务健康（需 JAVA_PLATFORM_PING=1）。
// 启用：JAVA_PLATFORM_PING=1，且 ETCD_ENDPOINTS 已配置、Java 服务已注册。
  func TestJavaPlatformContractPing(t *testing.T){
				   if os.Getenv("JAVA_PLATFORM_PING") != "1"{ 
		  t.Skip("set JAVA_PLATFORM_PING=1 to run java platform contract ping")
  } 
   if err := os.Chdir("../.."); err != nil{
		   t.Fatal(err)
		  }
	   app.LoadEnvFile(".env")
			   rt, err := app.Open()
			   if err != nil{
	  t.Fatalf("app open: %v", err)
			  }
   if rt.Dial == nil{
			  t.Fatal("svcdial unavailable")
		   } 
		  ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	  defer cancel()
			   client := platformjava.New( rt.Dial )
		   results := platformjava.PingCatalog(ctx,
		   client)
			  for _, r := range results{ 
		  if !r.OK() {
			  t.Logf("FAIL service=%s status=%d err=%v", r.Service, r.StatusCode, r.Err)
		   }
	   }
		  if err := platformjava.RequireAllOK(results); err != nil{
		   t.Fatal(err)
	   }
  }
