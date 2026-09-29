package stockwatch

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kaoqy/Nodeloc-Store/internal/models"
	catalogapp "github.com/kaoqy/Nodeloc-Store/internal/modules/catalog/application"
)

type fakeShelf struct {
	rows      []catalogapp.LowStockProduct
	err       error
	threshold int
	calls     int
}

func (f *fakeShelf) LowStockProducts(context.Context) ([]catalogapp.LowStockProduct, error) {
	f.calls++
	return f.rows, f.err
}

func (f *fakeShelf) AlertThreshold() int {
	if f.threshold > 0 {
		return f.threshold
	}
	return 5
}

type delivery struct {
	user    uint
	warning Warning
	since   time.Time
}

// fakeOutbox deduplicates the way the inbox does: an account, a link and a day
// make one message, and a later day is a message of its own.
type fakeOutbox struct {
	// warnedAt records, per account and link, the start of the day the warning
	// went out, so a sweep can only be silenced by a copy from that day or later.
	warnedAt map[string]time.Time
	sent     []delivery
	failFor  map[uint]error
}

func newFakeOutbox() *fakeOutbox {
	return &fakeOutbox{warnedAt: map[string]time.Time{}, failFor: map[uint]error{}}
}

func (o *fakeOutbox) Publish(_ context.Context, userID uint, warning Warning, since time.Time) (bool, error) {
	if err, broken := o.failFor[userID]; broken {
		return false, err
	}
	key := fmt.Sprintf("%d|%s", userID, warning.Link)
	if warned, already := o.warnedAt[key]; already && !warned.Before(since) {
		return false, nil
	}
	o.warnedAt[key] = since
	o.sent = append(o.sent, delivery{user: userID, warning: warning, since: since})
	return true, nil
}

type roster struct {
	ids []uint
	err error
	// calls counts how often the sweep had to look for people at all.
	calls int
}

func (r *roster) list(context.Context) ([]uint, error) {
	r.calls++
	return r.ids, r.err
}

func shelf(productID uint, name string, stock, sold int, waiting int64) catalogapp.LowStockProduct {
	return catalogapp.LowStockProduct{
		Product: models.Product{
			Base:       models.Base{ID: productID},
			Name:       name,
			StockCount: stock,
			SoldCount:  sold,
		},
		WaitingOrders: waiting,
	}
}

func fixedWatcher(rows ...catalogapp.LowStockProduct) (*Watcher, *fakeShelf, *fakeOutbox, *roster) {
	shelfFake := &fakeShelf{rows: rows}
	outbox := newFakeOutbox()
	people := &roster{ids: []uint{7}}
	watcher := New(shelfFake, outbox, people.list)
	watcher.now = func() time.Time { return time.Date(2026, 3, 5, 14, 30, 0, 0, time.Local) }
	return watcher, shelfFake, outbox, people
}

func TestPassWarnsEveryoneWhoRefills(t *testing.T) {
	watcher, _, outbox, people := fixedWatcher(shelf(11, "年付密钥", 2, 40, 0))
	people.ids = []uint{7, 8, 9}

	result, err := watcher.Pass(context.Background())
	if err != nil {
		t.Fatalf("Pass: %v", err)
	}
	if result.Checked != 1 || result.Sent != 3 {
		t.Errorf("result = %+v, want 1 shelf checked and 3 warnings sent", result)
	}
	for _, record := range outbox.sent {
		if record.warning.Link != "/cards/11" {
			t.Errorf("link = %q, want the product's own card page", record.warning.Link)
		}
		want := time.Date(2026, 3, 5, 0, 0, 0, 0, time.Local)
		if !record.since.Equal(want) {
			t.Errorf("since = %v, want the store's own midnight %v", record.since, want)
		}
	}
	if len(outbox.sent) != 3 {
		t.Fatalf("sent %+v, want one message per account", outbox.sent)
	}
	if outbox.sent[0].user == outbox.sent[1].user {
		t.Errorf("accounts repeated: %+v", outbox.sent)
	}
}

// A published product that has sold nothing and has nobody waiting is a shelf
// the shop has not opened yet; warning about it teaches nothing.
func TestPassSkipsShelvesNobodyHasOpened(t *testing.T) {
	watcher, _, outbox, people := fixedWatcher(
		shelf(1, "全新上架", 0, 0, 0),
		shelf(2, "有人等着", 0, 0, 2),
		shelf(3, "卖出过", 5, 12, 0),
	)

	result, err := watcher.Pass(context.Background())
	if err != nil {
		t.Fatalf("Pass: %v", err)
	}
	if result.Checked != 2 || result.Sent != 2 {
		t.Errorf("result = %+v, want the two shelves with demand", result)
	}
	if len(outbox.sent) != 2 {
		t.Fatalf("sent %d warnings, want 2", len(outbox.sent))
	}
	if people.calls != 1 {
		t.Errorf("roster read %d times, want one read for the whole pass", people.calls)
	}
}

// Nothing short at all must not even look for people to warn.
func TestPassStaysQuietWhenStockIsFine(t *testing.T) {
	watcher, _, outbox, people := fixedWatcher()

	result, err := watcher.Pass(context.Background())
	if err != nil {
		t.Fatalf("Pass: %v", err)
	}
	if result != (Result{}) {
		t.Errorf("result = %+v, want zero checked and zero sent", result)
	}
	if people.calls != 0 || len(outbox.sent) != 0 {
		t.Errorf("a healthy shop still got swept: roster %d, warnings %d", people.calls, len(outbox.sent))
	}
}

func TestWarningWordsFollowTheShelf(t *testing.T) {
	cases := []struct {
		name        string
		row         catalogapp.LowStockProduct
		wantTitle   string
		wantContent string
	}{
		{
			name:        "paid orders waiting",
			row:         shelf(4, "月付密钥", 0, 30, 3),
			wantTitle:   "库存预警：月付密钥 只剩 0 张",
			wantContent: "已有 3 笔已付款的订单在等这一件商品",
		},
		{
			name:        "empty but nobody waiting",
			row:         shelf(5, "季付密钥", 0, 8, 0),
			wantTitle:   "库存预警：季付密钥 只剩 0 张",
			wantContent: "下一笔付款会停在等待补货",
		},
		{
			name:        "still a few left",
			row:         shelf(6, "年付密钥", 3, 8, 0),
			wantTitle:   "库存预警：年付密钥 只剩 3 张",
			wantContent: "补货阈值 5 张。还剩 3 张",
		},
		{
			name:        "a product without a name",
			row:         shelf(7, "", 1, 4, 0),
			wantTitle:   "库存预警：商品 #7 只剩 1 张",
			wantContent: "补货阈值 5 张",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			warning := warningFor(testCase.row, 5)
			if warning.Title != testCase.wantTitle {
				t.Errorf("title = %q, want %q", warning.Title, testCase.wantTitle)
			}
			if !strings.Contains(warning.Content, testCase.wantContent) {
				t.Errorf("content = %q, want it to carry %q", warning.Content, testCase.wantContent)
			}
			if !strings.Contains(warning.Content, "补货阈值 5 张") {
				t.Errorf("content = %q, want the shop's threshold in it", warning.Content)
			}
		})
	}
}

// The whole point of the dedupe: sweeping again the same day must not turn a
// short shelf into a pile of identical messages.
func TestPassRepeatsNothingOnTheSameDay(t *testing.T) {
	row := shelf(9, "年付密钥", 0, 50, 1)
	watcher, _, outbox, _ := fixedWatcher(row)

	first, err := watcher.Pass(context.Background())
	if err != nil {
		t.Fatalf("first Pass: %v", err)
	}
	second, err := watcher.Pass(context.Background())
	if err != nil {
		t.Fatalf("second Pass: %v", err)
	}
	if first.Sent != 1 || second.Sent != 0 {
		t.Errorf("sent %d then %d, want the second sweep to add nothing", first.Sent, second.Sent)
	}
	if second.Checked != 1 {
		t.Errorf("checked = %d, want the shelf still reported as short", second.Checked)
	}
	if len(outbox.sent) != 1 {
		t.Fatalf("inbox holds %d messages, want 1", len(outbox.sent))
	}
}

// A day older is a new warning: the sweep must ask for today's copy, not for
// anything ever sent.
func TestPassAsksFromTheCurrentDay(t *testing.T) {
	watcher, _, outbox, _ := fixedWatcher(shelf(10, "月付密钥", 1, 5, 0))
	if _, err := watcher.Pass(context.Background()); err != nil {
		t.Fatalf("Pass: %v", err)
	}
	watcher.now = func() time.Time { return time.Date(2026, 3, 6, 0, 15, 0, 0, time.Local) }
	result, err := watcher.Pass(context.Background())
	if err != nil {
		t.Fatalf("next day Pass: %v", err)
	}
	if result.Sent != 1 {
		t.Errorf("sent %d, want yesterday's warning not to cover today", result.Sent)
	}
	want := time.Date(2026, 3, 6, 0, 0, 0, 0, time.Local)
	if !outbox.sent[len(outbox.sent)-1].since.Equal(want) {
		t.Errorf("since = %v, want %v", outbox.sent[len(outbox.sent)-1].since, want)
	}
}

func TestPassReportsTheSweepNotTheMailbox(t *testing.T) {
	queueDown := errors.New("the shelf list is unreadable")
	watcher, _, outbox, _ := fixedWatcher(shelf(12, "年付密钥", 0, 3, 1))
	watcher.shelf = &fakeShelf{err: queueDown}
	result, err := watcher.Pass(context.Background())
	if !errors.Is(err, queueDown) {
		t.Errorf("queue error = %v, want it handed back", err)
	}
	if result.Checked != 0 || len(outbox.sent) != 0 {
		t.Errorf("a failed read still warned somebody: %+v", result)
	}

	rosterDown := errors.New("accounts are unreadable")
	watcher, _, outbox, people := fixedWatcher(shelf(12, "年付密钥", 0, 3, 1))
	people.err = rosterDown
	if _, err = watcher.Pass(context.Background()); !errors.Is(err, rosterDown) {
		t.Errorf("roster error = %v, want it handed back", err)
	}
	if len(outbox.sent) != 0 {
		t.Errorf("warnings went out with no roster to address them")
	}
}

// One account whose inbox is down must not cost the others their warning.
func TestPassKeepsGoingAroundAFailedDelivery(t *testing.T) {
	broken := errors.New("inbox write refused")
	watcher, _, outbox, people := fixedWatcher(shelf(13, "年付密钥", 0, 9, 4))
	people.ids = []uint{1, 2, 3}
	outbox.failFor[2] = broken

	result, err := watcher.Pass(context.Background())
	if err != nil {
		t.Fatalf("Pass: %v", err)
	}
	if result.Sent != 2 {
		t.Errorf("sent = %d, want the two reachable accounts warned", result.Sent)
	}
	if len(outbox.sent) != 2 {
		t.Fatalf("delivered %+v, want 2 records", outbox.sent)
	}
}

// A shop with nobody allowed on the cards pages still gets its counts, and no
// error to show a user.
func TestPassWithoutAnyoneToWarn(t *testing.T) {
	watcher, _, outbox, people := fixedWatcher(shelf(14, "年付密钥", 0, 2, 1))
	people.ids = nil

	result, err := watcher.Pass(context.Background())
	if err != nil {
		t.Fatalf("Pass: %v", err)
	}
	if result.Checked != 1 || result.Sent != 0 || len(outbox.sent) != 0 {
		t.Errorf("result = %+v, want the shortage reported with nothing sent", result)
	}
}

func TestAlertMirrorsThePass(t *testing.T) {
	watcher, _, _, _ := fixedWatcher(shelf(15, "年付密钥", 1, 2, 0), shelf(16, "月付密钥", 0, 5, 1))
	checked, sent, err := watcher.Alert(context.Background())
	if err != nil {
		t.Fatalf("Alert: %v", err)
	}
	if checked != 2 || sent != 2 {
		t.Errorf("Alert = (%d, %d), want (2, 2) from the same pass", checked, sent)
	}
}

// Half a wired shop must not panic: a store can sell and restock with no
// watcher, and the sweep simply says nothing happened.
func TestPassToleratesMissingWiring(t *testing.T) {
	shelfFake := &fakeShelf{rows: []catalogapp.LowStockProduct{shelf(17, "年付密钥", 0, 1, 0)}}
	outbox := newFakeOutbox()
	people := &roster{ids: []uint{1}}

	for name, watcher := range map[string]*Watcher{
		"nil watcher":    nil,
		"no shelf":       New(nil, outbox, people.list),
		"no outbox":      New(shelfFake, nil, people.list),
		"no roster":      New(shelfFake, outbox, nil),
		"missing fields": {},
	} {
		t.Run(name, func(t *testing.T) {
			result, err := watcher.Pass(context.Background())
			if err != nil {
				t.Fatalf("Pass: %v", err)
			}
			if result != (Result{}) {
				t.Errorf("result = %+v, want an empty one", result)
			}
			if len(outbox.sent) != 0 {
				t.Errorf("an unwired shop sent %d warnings", len(outbox.sent))
			}
		})
	}
}
