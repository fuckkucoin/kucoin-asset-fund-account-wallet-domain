package flow

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/utabook"
	"github.com/shopspring/decimal"
)

   func TestUTABorrowInfoREST(t *testing.T) {
	  rt, h := router(t)
	  w := doJSON(t, rt, h, http.MethodGet, "/api/ua/v1/margin/borrow/info?currency=USDT", nil)
	  if w.Code != 200 {
		  t.Fatalf("borrow info %d %s", w.Code, w.Body.String())
	  }
	   var wrap struct {
		  Code string `json:"code"`
		   Data map[string]any `json:"data"`
	  }
	   if err := json.Unmarshal(w.Body.Bytes(), &wrap); err != nil {
		   t.Fatal(err)
	  }
	  if wrap.Code != "200000" {
		  t.Fatalf("code %s", wrap.Code)
	  }
  }

   func TestUTALiabilityAndRiskREST(t *testing.T) {
	  rt, h := router(t)
	   uid := seedUser(rt)
	   book := rt.UTA
	  book.UpsertLiability(uid, utabook.LiabilityRow{
		   Currency: "USDT", MarginMode: "CROSS", Principal: "10", Interest: "1",
	   })
	   book.PutCollateral(utabook.CollateralRule{Currency: "USDT", Haircut: "0.95", CollateralEnabled: true, MaxLTV: "0.8"})
	   book.UpsertRisk(uid, utabook.RiskView{UserID: uid, MarginLevel: "2.5", TotalEquity: "100", TotalLiability: "11", RiskGrade: "LOW"})
	   w := doJSON(t, rt, h, http.MethodGet, "/api/ua/v1/margin/borrow/liability?summary=1", nil)
	  if w.Code != 200 {
		  t.Fatalf("liability %d", w.Code)
	   }
	   w = doJSON(t, rt, h, http.MethodGet, "/api/ua/v1/account/risk-rate?units=1", nil)
	   if w.Code != 200 {
		   t.Fatalf("risk %d %s", w.Code, w.Body.String())
	  }
  }

   func TestUTARepayHistoryPaging(t *testing.T) {
	   rt, h := router(t)
	   uid := seedUser(rt)
	  rt.UTA.NoteRepay(uid, utabook.RepayRow{Currency: "USDT", Principal: "5", Interest: "0.1", RepayType: "REPAY"})
	  rt.UTA.NoteRepay(uid, utabook.RepayRow{Currency: "USDT", Principal: "3", Interest: "0", RepayType: "REPAY"})
	   w := doJSON(t, rt, h, http.MethodGet, "/api/ua/v1/margin/repay/history?page=1&pageSize=10", nil)
	  if w.Code != 200 {
		  t.Fatalf("repay %d", w.Code)
	  }
	  var wrap struct {
		   Data struct {
			   Total int `json:"total"`
		   } `json:"data"`
	  }
	  _ = json.Unmarshal(w.Body.Bytes(), &wrap)
	  if wrap.Data.Total < 2 {
		   t.Fatalf("want >=2 repay rows, got %d", wrap.Data.Total)
	   }
  }

  func TestUTACrossIsolatedMarginRead(t *testing.T) {
	  rt, h := router(t)
	   uid := seedUser(rt)
	  rt.UTA.UpsertCross(uid, utabook.CrossAsset{Currency: "USDT", Total: "100", Available: "90", Hold: "10", Liability: "5"})
	   w := doJSON(t, rt, h, http.MethodGet, "/api/ua/v1/margin/cross/account", nil)
	  if w.Code != 200 {
		  t.Fatalf("cross %d", w.Code)
	  }
	  w = doJSON(t, rt, h, http.MethodGet, "/api/ua/v1/margin/isolated/account?symbol=BTC-USDT", nil)
	   if w.Code != 200 {
		   t.Fatalf("isolated %d", w.Code)
	  }
   }

   func TestUTABalanceEnrichment(t *testing.T) {
	  rt, h := router(t)
	  uid := seedUser(rt)
	   rt.UTA.UpsertLiability(uid, utabook.LiabilityRow{Currency: "USDT", MarginMode: "CROSS", Principal: "2"})
	  rt.UTA.PutCollateral(utabook.CollateralRule{Currency: "USDT", CollateralEnabled: true, Haircut: "1"})
	   w := doJSON(t, rt, h, http.MethodGet, "/api/ua/v2/unified/account/balance", nil)
	  if w.Code != 200 {
		  t.Fatalf("balance %d %s", w.Code, w.Body.String())
	  }
	   var wrap struct {
		   Data struct {
			  Accounts []struct {
				   Currencies []map[string]any `json:"currencies"`
			   } `json:"accounts"`
		   } `json:"data"`
	  }
	   if err := json.Unmarshal(w.Body.Bytes(), &wrap); err != nil {
		  t.Fatal(err)
	  }
	   if len(wrap.Data.Accounts) == 0 || len(wrap.Data.Accounts[0].Currencies) == 0 {
		   t.Fatalf("empty balance payload")
	  }
	   found := false
	   for _, row := range wrap.Data.Accounts[0].Currencies {
		   if row["currency"] == "USDT" {
			  found = true
			  if row["liability"] == nil {
				   t.Fatalf("missing liability")
			   }
		  }
	   }
	  if !found {
		  t.Fatalf("USDT row not found")
	   }
  }

   func TestUTAGatewayRails(t *testing.T) {
	   rt, h := router(t)
	   uid := seedUser(rt)
	  rt.UTA.UpsertLiability(uid, utabook.LiabilityRow{Currency: "BTC", MarginMode: "CROSS", Principal: "0.01"})
	   paths := []string{
		   "/v1/assets/uta/liability",
		  "/v1/assets/uta/repay-history",
		  "/v1/assets/uta/risk",
		   "/v1/assets/uta/cross-margin",
		   "/v1/assets/uta/isolated-margin?symbol=ETH-USDT",
	   }
	   for _, p := range paths {
		  w := doJSON(t, rt, h, http.MethodGet, p, nil)
		   if w.Code != 200 {
			  t.Fatalf("%s => %d %s", p, w.Code, w.Body.String())
		   }
	  }
   }

   func TestUTABookRateCatalogFailClosed(t *testing.T) {
	  b := utabook.New()
	   lim := b.InterestLimits("USDT")
	   if lim.CurrentRateHourly != "0" || lim.BorrowLimitTotal != "0" {
		  t.Fatalf("unexpected fabricated limits %+v", lim)
	  }
	  b.RateCatalog().Put(utabook.RateTier{
		   Currency: "USDT", Tier: 0, RateHourly: "0.00001", RateDaily: "0.00024",
		   BorrowLimit: "10000", InterestFreeLimit: "100", HoldLimit: "0.8",
	  })
	  pot := b.RateCatalog().PotentialBorrow("u1", "USDT", "1000")
	  if pot == "0" {
		   t.Fatalf("expected positive potential borrow with configured ladder")
	  }
  }

   func TestUTABookSnapshotAndDescribe(t *testing.T) {
	  b := utabook.New()
	  uid := "user-test-uta"
	   b.SetMode(uid, "UNIFIED")
	   snap := b.Snapshot(uid)
	   if snap.Mode != "UNIFIED" {
		   t.Fatalf("mode %s", snap.Mode)
	  }
	  desc := b.DescribeCurrency(uid, "ETH")
	  if desc["currency"] != "ETH" {
		   t.Fatalf("describe %+v", desc)
	   }
   }

  func TestUTAInterestHistoryPagedREST(t *testing.T) {
	  rt, h := router(t)
	   uid := seedUser(rt)
	  rt.UTA.NoteInterest(uid, "USDT", "0.01")
	   w := doJSON(t, rt, h, http.MethodGet, "/api/ua/v1/account/interest-history-paged?page=1", nil)
	   if w.Code != 200 {
		   t.Fatalf("paged interest %d", w.Code)
	  }
  }

  func TestUTAMarginReadRouter(t *testing.T) {
	   rt, h := router(t)
	   w := doJSON(t, rt, h, http.MethodGet, "/api/ua/v1/margin/account?marginMode=CROSS&currency=USDT", nil)
	   if w.Code != 200 {
		  t.Fatalf("margin read %d", w.Code)
	   }
   }

   func TestUTARecomputeRisk(t *testing.T) {
	  rt, h := router(t)
	   uid := seedUser(rt)
	   rt.UTA.UpsertLiability(uid, utabook.LiabilityRow{Currency: "USDT", MarginMode: "CROSS", Principal: "50", Interest: "0"})
	  w := doJSON(t, rt, h, http.MethodGet, "/api/ua/v1/account/risk-rate?recompute=1&equity=200", nil)
	   if w.Code != 200 {
		  t.Fatalf("recompute %d", w.Code)
	  }
  }

  func TestUTABookKVRateCatalog(t *testing.T) {
	   rt := openMem(t)
	   book := rt.UTA
	  tiers := []utabook.RateTier{{Currency: "BTC", Tier: 0, RateHourly: "0", BorrowLimit: "1"}}
	   raw, err := json.Marshal(tiers)
	   if err != nil {
		   t.Fatal(err)
	  }
	   if err := rt.KV.Put(context.Background(), persist.Row{Kind: "aa_uta_rate", Key: "catalog_v1", Body: raw}, persist.Fence{}); err != nil {
		  t.Fatal(err)
	  }
	  book2 := utabook.New().BindKV(rt.KV)
	  if len(book2.Borrowable()) == 0 {
		  t.Fatal("borrowable empty after kv load")
	  }
	  _ = book.BindConfigTiers(tiers)
  }

   func TestUTABookEnrichCurrency(t *testing.T) {
	  b := utabook.New()
	  b.PutCollateral(utabook.CollateralRule{Currency: "USDC", CollateralEnabled: false})
	   en := b.EnrichCurrency("u", "USDC", "100", "100")
	   if en.CollateralStatus != "0" {
		  t.Fatalf("collateral status %s", en.CollateralStatus)
	   }
  }

   func TestUTABookTotalCrossLiability(t *testing.T) {
	  b := utabook.New()
	   uid := "u-cross"
	   b.UpsertCross(uid, utabook.CrossAsset{Currency: "USDT", Liability: "3"})
	   b.UpsertCross(uid, utabook.CrossAsset{Currency: "USDC", Liability: "2"})
	   if b.TotalCrossLiability(uid) != "5" {
		  t.Fatalf("total %s", b.TotalCrossLiability(uid))
	  }
  }

   func TestUTABookRepayFilterMarginMode(t *testing.T) {
	  rt := openMem(t)
	   uid := seedUser(rt)
	   rt.UTA.NoteRepay(uid, utabook.RepayRow{Currency: "USDT", MarginMode: "ISOLATED", Symbol: "BTC-USDT", Principal: "1"})
	   page := rt.UTA.RepayHistory(uid, utabook.RepayQuery{MarginMode: "ISOLATED", Page: 1, PageSize: 10})
	   if len(page.Items) != 1 {
		  t.Fatalf("items %d", len(page.Items))
	   }
  }

  func TestUTABookValidateBorrowReadClosed(t *testing.T) {
	   b := utabook.New()
	  if !b.ValidateBorrowReadClosed("USDT") {
		   t.Fatal("expected closed read for unset ladder")
	   }
   }

  func TestUTABookGatewayOverview(t *testing.T) {
	   b := utabook.New()
	   ov := b.GatewayOverview("u1")
	   if ov["userId"] != "u1" {
		  t.Fatalf("overview %+v", ov)
	  }
  }

  func TestUTABookLeverageUnchanged(t *testing.T) {
	  rt, h := router(t)
	  w := doJSON(t, rt, h, http.MethodGet, "/api/ua/v1/unified/account/leverage", nil)
	  if w.Code != 200 {
		  t.Fatalf("leverage %d", w.Code)
	  }
  }

  func TestUTABookAccrualProjection(t *testing.T) {
	  b := utabook.New()
	  b.PutRateTier(utabook.RateTier{Currency: "USDT", Tier: 0, RateHourly: "0.001", BorrowLimit: "1000"})
	   b.UpsertLiability("u", utabook.LiabilityRow{Currency: "USDT", MarginMode: "CROSS", Principal: "100"})
	   proj := b.ProjectAccrual("u", "USDT", decimal.NewFromInt(2))
	  if proj.Projected == "0" {
		  t.Fatalf("projection %+v", proj)
	  }
   }

   func TestUTABookMarginAggregate(t *testing.T) {
	   b := utabook.New()
	  b.UpsertCross("u", utabook.CrossAsset{Currency: "USDT", Liability: "10", Total: "100"})
	  agg := b.AggregateMargin("u", "BTC-USDT")
	  if agg.CrossLiability != "10" {
		   t.Fatalf("agg %+v", agg)
	   }
   }

   func TestUTABookRepayCSV(t *testing.T) {
	  b := utabook.New()
	  b.NoteRepay("u", utabook.RepayRow{Currency: "USDT", Principal: "1"})
	   raw, err := b.RepayHistoryCSV("u", utabook.RepayQuery{Page: 1, PageSize: 10})
	  if err != nil || len(raw) == 0 {
		   t.Fatalf("csv err=%v len=%d", err, len(raw))
	   }
  }

  func TestUTABookContractMap(t *testing.T) {
	  b := utabook.New()
	   q := b.ParseRepayQuery("usdt", "cross_margin", "", "2", "25", "", "")
	  if q.Currency != "USDT" || q.MarginMode != "CROSS" || q.Page != 2 {
		   t.Fatalf("query %+v", q)
	   }
	   resp := b.BorrowInfoResponse("ETH")
	  if resp.Currency != "ETH" {
		   t.Fatalf("resp %+v", resp)
	   }
  }
