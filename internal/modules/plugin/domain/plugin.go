package domain

import (
	"errors"
	"sort"
	"strings"

	"github.com/kaoqy/Nodeloc-Store/internal/models"
)

// Plugin and PluginBinding are the canonical persistence models. The module
// keeps its own aliases so callers never reach into internal/models directly.
type (
	Plugin        = models.Plugin
	PluginBinding = models.PluginBinding
)

var (
	ErrPluginNotFound  = errors.New("plugin not found")
	ErrPluginDisabled  = errors.New("plugin is disabled")
	ErrBindingNotFound = errors.New("plugin binding not found")
	ErrInvalidInput    = errors.New("invalid plugin input")
	// ErrChannelNotReady marks a delivery channel the shop has not finished
	// configuring. Checkout refuses before money moves, and the message the
	// buyer sees says so without leaking the missing credential names.
	ErrChannelNotReady = errors.New("delivery channel is not configured")
	// ErrNoBinding is the buyer-facing answer when a purchase needs a plugin
	// mapping and none exists: better a sentence than delivering the wrong thing.
	ErrNoBinding = errors.New("这个商品还没有匹配到对应的交付项目")
)

// Capability names what a plugin may contribute. A provider declares the set it
// supports; the back office and the storefront read the same list, so a plugin
// that cannot fulfil orders is never offered where fulfilment is expected.
const (
	// CapabilityForm lets the plugin contribute purchase-form fields.
	CapabilityForm = "form"
	// CapabilityFulfill lets the plugin resolve and deliver an order.
	CapabilityFulfill = "fulfill"
	// CapabilityNotify lets the plugin emit buyer notifications.
	CapabilityNotify = "notify"
)

// AllCapabilities is the vocabulary the settings screen offers, in display order.
var AllCapabilities = []string{CapabilityForm, CapabilityFulfill, CapabilityNotify}

func ValidCapability(value string) bool {
	for _, candidate := range AllCapabilities {
		if candidate == value {
			return true
		}
	}
	return false
}

// NormalizeCapabilities trims, lowercases, drops duplicates and unknown names,
// and returns them in the canonical order so two saves of the same set compare
// equal.
func NormalizeCapabilities(values []string) []string {
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value != "" && ValidCapability(value) {
			seen[value] = true
		}
	}
	out := make([]string, 0, len(seen))
	for _, candidate := range AllCapabilities {
		if seen[candidate] {
			out = append(out, candidate)
		}
	}
	return out
}

// NormalizeBindingValue is the key two bindings are compared by. It is trimmed
// and lowercased so "北京" and " 北京 " resolve alike, and so an English option
// does not depend on the buyer's shift key.
func NormalizeBindingValue(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

// SortBindings orders a product's mappings by the owner's sort order, then by
// value, so the list does not reshuffle between reads.
func SortBindings(bindings []PluginBinding) {
	sort.SliceStable(bindings, func(i, j int) bool {
		if bindings[i].SortOrder != bindings[j].SortOrder {
			return bindings[i].SortOrder < bindings[j].SortOrder
		}
		return bindings[i].Value < bindings[j].Value
	})
}
