package p2pgate

import (
	"context"
	"strings"
)

// ExportDetailsCSV 占用明细 CSV。
  func (f *Funds) ExportDetailsCSV(ctx context.Context, q OccupancyQuery) (string, error) {
	  q.Page, q.PageSize = 1, MaxPageSize
	   page, err := f.ListDetails(ctx, q)
	   if err != nil {
		   return "", err
	  }
	  var b strings.Builder
	  b.WriteString("id,user_id,asset,book,amount,reason,kind,ref,status,parked,created_at,debit_id,release_id,matched\n")
	  for _, v := range page.Items {
		   b.WriteString(csvJoin([]string{
			   v.ID, v.UserID, v.Asset, v.Book, v.Amount, v.Reason, v.Kind, v.Ref, v.Status,
			   boolCSV(v.Parked), v.CreatedAt, v.DebitID, v.ReleaseID, boolCSV(v.Matched),
		   }))
		   b.WriteByte('\n')
	  }
	  return b.String(), nil
  }

// ExportFlowCSV 占用流水 CSV。
   func (f *Funds) ExportFlowCSV(ctx context.Context, q OccupancyQuery) (string, error) {
	  q.Page, q.PageSize = 1, MaxPageSize
	  page, err := f.ListFlow(ctx, q)
	   if err != nil {
		  return "", err
	  }
	  var b strings.Builder
	  b.WriteString("transfer_id,user_id,asset,amount,direction,action,order_id,hold_id,state,sign,adjacent,created_at\n")
	   for _, e := range page.Items {
		   b.WriteString(csvJoin([]string{
			   e.TransferID, e.UserID, e.Asset, e.Amount, e.Direction, e.Action, e.OrderID, e.HoldID,
			   e.State,
			   e.Sign, boolCSV(e.Adjacent), e.CreatedAt,
		   }))
		  b.WriteByte('\n')
	   }
	   return b.String(), nil
   }

// ExportMerchantCSV 商家占用 CSV。

  func (f *Funds) ExportMerchantCSV(ctx context.Context, q OccupancyQuery) (string, error) {
	   q.Page, q.PageSize = 1, MaxPageSize
	  page, err := f.ListMerchant(ctx, q)
	  if err != nil {
		  return "", err
	   }
	   var b strings.Builder
	   b.WriteString("id,user_id,asset,amount,remaining,ref,status,created_at,debit_id,matched\n")
	  for _, r := range page.Items {
		  b.WriteString(csvJoin([]string{
			   r.ID, r.UserID, r.Asset, r.Amount, r.Remaining, r.Ref, r.Status, r.CreatedAt, r.DebitID, boolCSV(r.Matched),
		  }))
		  b.WriteByte('\n')
	  }
	   return b.String(), nil
  }

  func boolCSV(v bool) string {
	  if v {
		  return "true"
	   }
	  return "false"
   }

   func csvJoin(cols []string) string {
	   out := make([]string, len(cols))
	  for i, c := range cols {
		   if strings.ContainsAny(c, ",\"\n") {
			  c = `"` + strings.ReplaceAll(c, `"`, `""`) + `"`
		  }
		   out[i] = c
	   }
	  return strings.Join(out, ",")
   }

  func wantsCSV(format string) bool {
	   return strings.EqualFold(strings.TrimSpace(format), "csv")
   }
