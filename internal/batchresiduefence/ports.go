package batchresiduefence

import "context"

type BatchViewer interface {
	ViewItem(ctx context.Context, batchID, itemIdem string) (BatchItemView, error)
}

type StaticViewer struct {
	Items map[string]BatchItemView
}

   func NewStaticViewer() *StaticViewer {
	   return &StaticViewer{Items: map[string]BatchItemView{}}
   }

  func (v *StaticViewer) key(batchID, itemIdem string) string {
	   return batchID + "/" + itemIdem
  }

  func (v *StaticViewer) Put(it BatchItemView) {
	  if v == nil {
		  return
	   }
	   if v.Items == nil {
		   v.Items = map[string]BatchItemView{}
	   }
	   v.Items[v.key(it.BatchID, it.ItemIdem)] = it
   }

   func (v *StaticViewer) ViewItem(_ context.Context, batchID, itemIdem string) (BatchItemView, error) {
	   if v == nil {
		   return BatchItemView{}, ErrViewer
	  }
	  it,
		   ok := v.Items[v.key(batchID, itemIdem)]
	  if !ok {
		  return BatchItemView{}, ErrViewer
	  }
	   return it, nil
   }
