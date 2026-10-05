package application

import (
	"context"
	"errors"
	"testing"

	"github.com/kaoqy/Nodeloc-Store/internal/config"
	"github.com/kaoqy/Nodeloc-Store/internal/models"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/payment/contract"
)

type reviewProductRepo struct {
	contract.OrderRepo
	order *models.Order
}

func (r *reviewProductRepo) GetOrderByNo(context.Context, string) (*models.Order, error) {
	return r.order, nil
}

func (r *reviewProductRepo) MarkOrderPluginDelivering(context.Context, string) error {
	r.order.FulfillmentStatus = "plugin_pending"
	return nil
}

func (r *reviewProductRepo) MarkOrderPluginReview(_ context.Context, _ string, note string) error {
	r.order.FulfillmentStatus = "plugin_review"
	r.order.DeliveryNote = &note
	return nil
}

func (r *reviewProductRepo) MarkOrderPluginDelivered(context.Context, string, string, string) error {
	r.order.FulfillmentStatus = "delivered"
	return nil
}

type reviewPluginDeliverer struct {
	contract.PluginDeliverer
	calls int
}

// blockingPluginDeliverer signals when it has been entered and waits, so a test
// can observe the order's state while the external call is in flight.
type blockingPluginDeliverer struct {
	contract.PluginDeliverer
	entered chan struct{}
	release chan struct{}
	calls   int
}

func (d *blockingPluginDeliverer) Owns(context.Context, *models.Order) (bool, error) {
	return true, nil
}

func (d *blockingPluginDeliverer) Fulfill(context.Context, *models.Order) (*contract.PluginDelivery, error) {
	d.calls++
	close(d.entered)
	<-d.release
	return &contract.PluginDelivery{Content: "code-abc", Note: "ok"}, nil
}

// While the upstream redemption call is in flight the order must read as
// "generating", so the buyer sees progress instead of a blank waiting state.
func TestOrderShowsGeneratingWhileUpstreamInFlight(t *testing.T) {
	repo := &reviewProductRepo{order: &models.Order{
		OrderNo:           "NL-GEN-1",
		ProductID:         3,
		FulfillmentStatus: "pending",
		Status:            "paid",
	}}
	deliverer := &blockingPluginDeliverer{entered: make(chan struct{}), release: make(chan struct{})}
	service := &Service{orders: repo, fulfillment: &reviewFulfillment{}, features: config.FeaturesConfig{}}
	service.SetPluginDeliverer(deliverer)

	done := make(chan struct{})
	go func() {
		_, _ = service.FulfillOrder(context.Background(), "NL-GEN-1")
		close(done)
	}()

	<-deliverer.entered
	if repo.order.FulfillmentStatus != "plugin_pending" {
		t.Fatalf("in-flight status = %q, want plugin_pending (generating)", repo.order.FulfillmentStatus)
	}
	close(deliverer.release)
	<-done
}

func (d *reviewPluginDeliverer) Owns(context.Context, *models.Order) (bool, error) {
	return true, nil
}

func (d *reviewPluginDeliverer) Fulfill(context.Context, *models.Order) (*contract.PluginDelivery, error) {
	d.calls++
	return &contract.PluginDelivery{
		Content:   "pending review",
		Note:      "结果待确认",
		Uncertain: true,
	}, nil
}

type reviewFulfillment struct {
	contract.FulfillmentService
	calls int
}

func (f *reviewFulfillment) Fulfill(context.Context, *models.Order) error {
	f.calls++
	return nil
}

func TestUncertainPluginDeliveryIsParkedForReview(t *testing.T) {
	order := &models.Order{
		Base:              models.Base{ID: 1},
		OrderNo:           "NL1",
		ProductID:         5,
		Status:            "paid",
		FulfillmentStatus: "pending",
	}
	repo := &reviewProductRepo{order: order}
	plugins := &reviewPluginDeliverer{}
	fallback := &reviewFulfillment{}
	service := &Service{
		orders:      repo,
		plugins:     plugins,
		fulfillment: fallback,
		users:       orderUserDirectory{},
		features:    config.FeaturesConfig{},
	}

	if _, err := service.deliverOrder(context.Background(), order); err != nil {
		t.Fatalf("deliverOrder: %v", err)
	}
	if plugins.calls != 1 {
		t.Fatalf("plugin called %d times, want 1", plugins.calls)
	}
	if order.FulfillmentStatus != "plugin_review" {
		t.Fatalf("status = %q, want plugin_review", order.FulfillmentStatus)
	}
	if fallback.calls != 0 {
		t.Fatalf("fallback fulfillment ran for an uncertain plugin delivery")
	}
}

func TestUncertainPluginDeliveryReturnsErrorOnlyForHardFailures(t *testing.T) {
	// A hard provider error must stay a retryable error; only a sent request with
	// an unreadable answer becomes plugin_review.
	order := &models.Order{Base: models.Base{ID: 1}, OrderNo: "NL2", ProductID: 5}
	repo := &reviewProductRepo{order: order}
	plugins := &reviewErrorDeliverer{err: errors.New("provider down")}
	service := &Service{
		orders:      repo,
		plugins:     plugins,
		fulfillment: &reviewFulfillment{},
		users:       orderUserDirectory{},
	}
	if _, err := service.deliverOrder(context.Background(), order); err == nil {
		t.Fatal("hard provider failure was not returned")
	}
	if order.FulfillmentStatus == "plugin_review" {
		t.Fatal("hard provider failure was parked for review instead of retried")
	}
}

func TestReviewRetryRequiresExplicitConfirmation(t *testing.T) {
	order := &models.Order{
		Base:              models.Base{ID: 1},
		OrderNo:           "NL3",
		ProductID:         5,
		Status:            "paid",
		FulfillmentStatus: "plugin_review",
	}
	repo := &reviewProductRepo{order: order}
	plugins := &reviewPluginDeliverer{}
	service := &Service{
		orders:      repo,
		plugins:     plugins,
		fulfillment: &reviewFulfillment{},
		users:       orderUserDirectory{},
	}
	if _, err := service.FulfillOrderConfirmed(context.Background(), order.OrderNo, false); err == nil {
		t.Fatal("review retry without confirmation was accepted")
	}
	if plugins.calls != 0 {
		t.Fatalf("unconfirmed review retry called provider %d times", plugins.calls)
	}
}

type reviewErrorDeliverer struct {
	contract.PluginDeliverer
	err error
}

func (d *reviewErrorDeliverer) Owns(context.Context, *models.Order) (bool, error) {
	return true, nil
}

func (d *reviewErrorDeliverer) Fulfill(context.Context, *models.Order) (*contract.PluginDelivery, error) {
	return nil, d.err
}
