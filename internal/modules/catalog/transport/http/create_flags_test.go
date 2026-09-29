package http

import (
	"encoding/json"
	"testing"
)

// onUnlessTurnedOff keeps the meaning the columns used to carry: a create
// payload that never mentions the switch means "on".
func TestOnUnlessTurnedOff(t *testing.T) {
	if !onUnlessTurnedOff(nil) {
		t.Error("an unmentioned switch must arrive on, the way the old column default left it")
	}
	on, off := true, false
	if !onUnlessTurnedOff(&on) {
		t.Error("an explicit true has to stay true")
	}
	if onUnlessTurnedOff(&off) {
		t.Error("an explicit false must reach the row as false instead of becoming true")
	}
}

// The create inputs lean on Go's JSON rule that a field declared on the outer
// struct hides the same-named field promoted from the embedded row. If that ever
// stops holding, an omitted "is_published" would land as false and the shop would
// be creating products nobody can see.
func TestCreateInputsTellAbsentFromOff(t *testing.T) {
	var silent productInput
	if err := json.Unmarshal([]byte(`{"slug":"a","name":"A","price":1}`), &silent); err != nil {
		t.Fatal(err)
	}
	if silent.StockVisible != nil || silent.IsPublished != nil {
		t.Fatalf("unmentioned flags must stay nil, got %v / %v", silent.StockVisible, silent.IsPublished)
	}
	if silent.Product.Slug != "a" || silent.Product.Price != 1 {
		t.Errorf("the row itself stopped decoding: %+v", silent.Product)
	}

	off := productInput{}
	if err := json.Unmarshal([]byte(`{"slug":"b","is_published":false,"stock_visible":false}`), &off); err != nil {
		t.Fatal(err)
	}
	if onUnlessTurnedOff(off.IsPublished) || onUnlessTurnedOff(off.StockVisible) {
		t.Error("a payload that switched both off must stay off")
	}

	category := categoryInput{}
	if err := json.Unmarshal([]byte(`{"slug":"c","name":"C","is_visible":false}`), &category); err != nil {
		t.Fatal(err)
	}
	if onUnlessTurnedOff(category.IsVisible) {
		t.Error("a hidden category must stay hidden")
	}
	untouched := categoryInput{}
	if err := json.Unmarshal([]byte(`{"slug":"d","name":"D"}`), &untouched); err != nil {
		t.Fatal(err)
	}
	if !onUnlessTurnedOff(untouched.IsVisible) {
		t.Error("a category created without saying arrives visible, as it always did")
	}

	coupon := couponInput{}
	if err := json.Unmarshal([]byte(`{"code":"D","discount_type":"fixed","discount_value":5,"is_active":false}`), &coupon); err != nil {
		t.Fatal(err)
	}
	if onUnlessTurnedOff(coupon.IsActive) {
		t.Error("a coupon created switched off must stay switched off")
	}
	byDefault := couponInput{}
	if err := json.Unmarshal([]byte(`{"code":"E","discount_type":"fixed","discount_value":5}`), &byDefault); err != nil {
		t.Fatal(err)
	}
	if !onUnlessTurnedOff(byDefault.IsActive) {
		t.Error("a coupon created without saying arrives ready to use")
	}
}
