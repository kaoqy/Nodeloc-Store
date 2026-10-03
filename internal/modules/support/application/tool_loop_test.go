package application

import "testing"

// TestParseToolCall pins the contract the model is told to follow: one line of
// JSON. Without a parser the model's tool requests would be answered as plain
// text, which is exactly why the AI could not use any tool before.
func TestParseToolCall(t *testing.T) {
	call := parseToolCall(`好的，我查一下。
{"tool":"order.detail","params":{"order_no":"NL20260101001"}}`)
	if call == nil {
		t.Fatal("tool call not parsed")
	}
	if call.ToolKey != "order.detail" {
		t.Fatalf("tool = %q", call.ToolKey)
	}
	if call.Params["order_no"] != "NL20260101001" {
		t.Fatalf("params = %#v", call.Params)
	}
}

// TestParseToolCallIgnoresProse keeps normal answers untouched: only the exact
// shape counts, so a curly brace in conversation is not mistaken for a call.
func TestParseToolCallIgnoresProse(t *testing.T) {
	for _, text := range []string{
		"你的订单已经发货了。",
		"请提供订单号，格式如 {NL123}。",
		`{"order_no":"NL1"}`,
	} {
		if parseToolCall(text) != nil {
			t.Fatalf("prose was treated as a tool call: %q", text)
		}
	}
}

// TestStripToolCallRemovesOnlyTheCall keeps the explanation the model wrote
// around the call, so the buyer still sees a sentence.
func TestStripToolCallRemovesOnlyTheCall(t *testing.T) {
	got := stripToolCall("我帮你查一下。\n{\"tool\":\"order.list\",\"params\":{}}")
	if got != "我帮你查一下。" {
		t.Fatalf("strip = %q", got)
	}
}

// TestParseToolCallEmptyParams accepts a tool that takes no arguments.
func TestParseToolCallEmptyParams(t *testing.T) {
	call := parseToolCall(`{"tool":"user.profile","params":{}}`)
	if call == nil || call.ToolKey != "user.profile" {
		t.Fatalf("call = %#v", call)
	}
	if len(call.Params) != 0 {
		t.Fatalf("params = %#v", call.Params)
	}
}
