// Package stockwatch turns the shop's restocking queue into messages the people
// who refill the shelves actually receive. The queue itself is the catalogue's
// business; this package only answers "who needs to know, and have they already
// been told today".
package stockwatch

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	catalogapp "github.com/kaoqy/Nodeloc-Store/internal/modules/catalog/application"
)

// Warning is one shelf the shop has to refill, already in the words the back
// office inbox shows it in.
type Warning struct {
	Title   string
	Content string
	Link    string
}

// Shelf is the restocking queue the catalogue keeps: published card products at
// or below the shop's own threshold, with the paid orders waiting on each one.
type Shelf interface {
	LowStockProducts(ctx context.Context) ([]catalogapp.LowStockProduct, error)
	AlertThreshold() int
}

// Outbox delivers one warning to one account and reports whether that was new
// work. `since` is where the current day began, so yesterday's copy never
// counts as having covered today.
type Outbox interface {
	Publish(ctx context.Context, userID uint, warning Warning, since time.Time) (bool, error)
}

// Result is one pass, counted the way the back office reports it: how many
// shelves were short, and how many messages that turned into.
type Result struct {
	Checked int `json:"checked"`
	Sent    int `json:"sent"`
}

// Watcher walks the queue and leaves one warning per account per product per day.
type Watcher struct {
	shelf  Shelf
	outbox Outbox
	// roster is "who refills shelves": the accounts the container resolves for
	// the cards pages. A roster that cannot be read is reported, not guessed at.
	roster func(context.Context) ([]uint, error)
	now    func() time.Time
}

// New builds a watcher over the catalogue queue, an outbox and a roster. Any of
// them being absent leaves the pass silent: a store can sell, restock and
// deliver without this, and a missing wire must not read as an empty shop.
func New(shelf Shelf, outbox Outbox, roster func(context.Context) ([]uint, error)) *Watcher {
	return &Watcher{shelf: shelf, outbox: outbox, roster: roster, now: time.Now}
}

// Pass runs one sweep and returns what it found. A single delivery that fails
// costs that one message only: the shelf is still short, and the next pass has
// the same thing to say about it.
func (w *Watcher) Pass(ctx context.Context) (Result, error) {
	var out Result
	if w == nil || w.shelf == nil || w.outbox == nil || w.roster == nil {
		return out, nil
	}
	rows, err := w.shelf.LowStockProducts(ctx)
	if err != nil {
		return out, err
	}
	// A shelf that has never sold anything is a shelf the shop has not opened
	// yet; warning about it would only train the owner to ignore warnings.
	short := make([]catalogapp.LowStockProduct, 0, len(rows))
	for _, row := range rows {
		if row.WaitingOrders > 0 || row.SoldCount > 0 {
			short = append(short, row)
		}
	}
	out.Checked = len(short)
	if len(short) == 0 {
		return out, nil
	}
	accounts, err := w.roster(ctx)
	if err != nil {
		return out, err
	}
	if len(accounts) == 0 {
		log.Printf("stockwatch: %d product(s) need restocking but no account may open the cards pages", len(short))
		return out, nil
	}
	threshold := w.shelf.AlertThreshold()
	start := dayStart(w.now())
	for _, row := range short {
		warning := warningFor(row, threshold)
		for _, account := range accounts {
			fresh, err := w.outbox.Publish(ctx, account, warning, start)
			if err != nil {
				log.Printf("stockwatch: warning user %d about product %d: %v", account, row.ID, err)
				continue
			}
			if fresh {
				out.Sent++
			}
		}
	}
	return out, nil
}

// Alert is one sweep in the shape a route can answer with.
func (w *Watcher) Alert(ctx context.Context) (int, int, error) {
	result, err := w.Pass(ctx)
	return result.Checked, result.Sent, err
}

func warningFor(row catalogapp.LowStockProduct, threshold int) Warning {
	name := strings.TrimSpace(row.Name)
	if name == "" {
		name = fmt.Sprintf("商品 #%d", row.ID)
	}
	content := fmt.Sprintf("补货阈值 %d 张。", threshold)
	switch {
	case row.WaitingOrders > 0:
		content += fmt.Sprintf("已有 %d 笔已付款的订单在等这一件商品，导入卡密后会立刻自动发货。", row.WaitingOrders)
	case row.StockCount <= 0:
		content += "货架已经空了，下一笔付款会停在等待补货。"
	default:
		content += fmt.Sprintf("还剩 %d 张，卖完就要手工补货。", row.StockCount)
	}
	return Warning{
		Title:   fmt.Sprintf("库存预警：%s 只剩 %d 张", name, row.StockCount),
		Content: content,
		Link:    fmt.Sprintf("/cards/%d", row.ID),
	}
}

// dayStart is the store's own midnight, so "today" means the day the owner is
// looking at rather than a UTC boundary in the middle of it.
func dayStart(now time.Time) time.Time {
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
}
