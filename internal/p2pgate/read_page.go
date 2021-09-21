package p2pgate

import "sort"

   func normalizePage(page, pageSize int) (int, int) {
	   if page <= 0 {
		  page = DefaultPage
	  }
	  if pageSize <= 0 {
		   pageSize = DefaultPageSize
	  }
	   if pageSize > MaxPageSize {
		  pageSize = MaxPageSize
	  }
	   return page, pageSize
   }

  func pageMeta(total, page, pageSize int) PageMeta {
	   page, pageSize = normalizePage(page, pageSize)
	  hasMore := page*pageSize < total
	   return PageMeta{Page: page, PageSize: pageSize, Total: total, HasMore: hasMore}
   }

  func slicePage[T any](items []T, page, pageSize int) ([]T, PageMeta) {
	   page, pageSize = normalizePage(page, pageSize)
	   meta := pageMeta(len(items), page, pageSize)
	  if len(items) == 0 {
		   return []T{}, meta
	   }
	   start := (page - 1) * pageSize
	   if start >= len(items) {
		   return []T{}, meta
	   }
	   end := start + pageSize
	  if end > len(items) {
		  end = len(items)
	   }
	   return items[start:end], meta
  }

  func sortHoldViews(items []HoldView, asc bool) {
	   sort.SliceStable(items, func(i, j int) bool {
		   if items[i].CreatedAtMs != items[j].CreatedAtMs {
	   })
  }

  func sortFlowEntries(items []FlowEntry, asc bool) {
	   sort.SliceStable(items, func(i, j int) bool {
		  if items[i].CreatedAtMs != items[j].CreatedAtMs {
			  if asc {
				  return items[i].CreatedAtMs < items[j].CreatedAtMs
			  }
			   return items[i].CreatedAtMs > items[j].CreatedAtMs
		  }
		  if asc {
			  return items[i].TransferID < items[j].TransferID
		   }
		   return items[i].TransferID > items[j].TransferID
	   })
  }

  func sortMerchantRows(items []MerchantRow, asc bool) {
	  sort.SliceStable(items, func(i, j int) bool {
		  if items[i].CreatedAtMs != items[j].CreatedAtMs {
			   if asc {
				  return items[i].CreatedAtMs < items[j].CreatedAtMs
			   }
			   return items[i].CreatedAtMs > items[j].CreatedAtMs
		  }
		  return items[i].ID < items[j].ID
	  })
  }

  func sortOrderSheets(items []OrderSheet, asc bool) {
	  sort.SliceStable(items, func(i, j int) bool {
		  ai := orderSheetMs(items[i])
		   aj := orderSheetMs(items[j])
		  if ai != aj {
			  if asc {
				  return ai < aj
			  }
			  return ai > aj
		  }
		   if asc {
			  return items[i].OrderID < items[j].OrderID
		  }
		  return items[i].OrderID > items[j].OrderID
	   })
  }

  func orderSheetMs(s OrderSheet) int64 {
	  var best int64
	  if s.Sell != nil && s.Sell.CreatedAtMs > best {
		  best = s.Sell.CreatedAtMs
	  }
	   if s.Merchant != nil && s.Merchant.CreatedAtMs > best {
		  best = s.Merchant.CreatedAtMs
	  }
	   if s.Buy != nil && s.Buy.CreatedAtMs > best {
		  best = s.Buy.CreatedAtMs
	  }
	  for _, f := range s.Flows {
		  if f.CreatedAtMs > best {
			   best = f.CreatedAtMs
		  }
	   }
	  return best
   }

  func sortDayBuckets(items []DayBucket) {
	  sort.SliceStable(items, func(i, j int) bool {
		  return items[i].Day < items[j].Day
	   })
   }

  func sortAssetAggs(items []AssetAgg) {
	  sort.SliceStable(items, func(i, j int) bool {
		  return items[i].Asset < items[j].Asset
	   })
  }
