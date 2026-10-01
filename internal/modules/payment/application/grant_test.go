package application

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/kaoqy/Nodeloc-Store/internal/modules/payment/contract"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/payment/domain"
)

// grantLedger is the transfer ledger only. Embedding the repo interface keeps the
// fake about the one thing 转账 does to storage — write a row either way.
type grantLedger struct {
	contract.OrderRepo
	created []*domain.Transfer
	err     error
}

func (l *grantLedger) CreateTransfer(_ context.Context, transfer *domain.Transfer) error {
	l.created = append(l.created, transfer)
	return l.err
}

type grantDirectory struct {
	contract.UserLookup
	byID map[uint]*contract.UserInfo
}

func (d *grantDirectory) FindByID(_ context.Context, id uint) (*contract.UserInfo, error) {
	return d.byID[id], nil
}

type grantProvider struct {
	contract.PaymentGateway
	requests []contract.TransferRequest
	result   *contract.TransferResult
	err      error
}

func (p *grantProvider) Transfer(_ context.Context, request contract.TransferRequest) (*contract.TransferResult, error) {
	p.requests = append(p.requests, request)
	return p.result, p.err
}

func grantService(provider *grantProvider, users map[uint]*contract.UserInfo) (*Service, *grantLedger, *recordedNotifier) {
	ledger := &grantLedger{}
	events := &recordedNotifier{}
	return &Service{
		orders:    ledger,
		gateway:   provider,
		users:     &grantDirectory{byID: users},
		events:    events,
		paymentID: "pay_test",
	}, ledger, events
}

func boundUsers() map[uint]*contract.UserInfo {
	return map[uint]*contract.UserInfo{
		7: {ID: 7, Username: "buyer_one", IsActive: true, OAuthUID: "4242", OAuthUsername: "buyer_one_nl"},
		8: {ID: 8, Username: "ghost", IsActive: true},
		9: {ID: 9, Username: "suspended", IsActive: false, OAuthUID: "9"},
		1: {ID: 1, Username: "owner", IsActive: true},
		2: {ID: 2, Username: "gone", IsActive: false},
	}
}

// rejected builds the error the real gateway hands back for a NodeLoc refusal,
// sentinel included: classifyGrantError only translates answers the provider
// actually gave, and a bare string is not one of those.
func rejected(message string) error {
	return fmt.Errorf("%w: NodeLoc returned HTTP 200: %s", domain.ErrProviderRejected, message)
}

func TestClassifyGrantErrorWordsEachDocumentedRefusal(t *testing.T) {
	cases := []struct {
		provider   string
		wantCode   string
		wantStatus int
		wantPhrase string
	}{
		{"Receiver id and username do not match", "grant_recipient_mismatch", 409, "收款方对不上"},
		{"Cannot transfer to yourself", "grant_self", 409, "自己给自己转"},
		{"Amount below minimum limit (10)", "grant_below_minimum", 400, "限额为 10"},
		{"Amount exceeds maximum limit (2000)", "grant_above_maximum", 400, "限额为 2000"},
		{"Transfer feature is disabled", "grant_disabled", 409, "没有开启转账功能"},
		{"Invalid signature", "grant_signature", 502, "Payment Token"},
		{"Invalid amount", "grant_amount", 400, "整数"},
		{"Order already exists with status completed", "grant_reference_exists", 409, "不要重复转出"},
		{"Something nobody documented", "grant_rejected", 502, "Something nobody documented"},
	}
	for _, tc := range cases {
		failure := classifyGrantError(rejected(tc.provider))
		if failure.Code != tc.wantCode {
			t.Errorf("%q: got code %q, want %q", tc.provider, failure.Code, tc.wantCode)
		}
		if failure.Status != tc.wantStatus {
			t.Errorf("%q: got status %d, want %d", tc.provider, failure.Status, tc.wantStatus)
		}
		if !strings.Contains(failure.Message, tc.wantPhrase) {
			t.Errorf("%q: message %q does not say %q", tc.provider, failure.Message, tc.wantPhrase)
		}
		// The owner reads their own language first, but NodeLoc's sentence has to
		// stay reachable — it is what gets quoted back to the provider.
		if !strings.Contains(failure.Message, tc.provider) && tc.wantCode != "grant_rejected" {
			t.Errorf("%q: the provider's own words were dropped from %q", tc.provider, failure.Message)
		}
	}
}

// The gateway reads 「Order already exists with status …」 into the typed duplicate
// refusal before anyone sees the provider's sentence, so the 转账 path has to
// classify that type — matching on the English text only works when the gateway
// did not translate it first, which is how a duplicate 转账 ended up reported as
// an unexplained provider rejection.
func TestClassifyGrantErrorReadsTheTypedDuplicateRefusal(t *testing.T) {
	failure := classifyGrantError(&domain.PaymentAlreadyRequested{Status: "completed"})
	if failure.Code != "grant_reference_exists" || failure.Status != 409 {
		t.Fatalf("got %q/%d, want grant_reference_exists/409", failure.Code, failure.Status)
	}
	if !strings.Contains(failure.Message, "已经收过") || !strings.Contains(failure.Message, "不要重复转出") {
		t.Errorf("message %q", failure.Message)
	}
	if strings.Contains(failure.Message, "already has a payment") {
		t.Errorf("our own sentinel leaked to the shop owner: %q", failure.Message)
	}
}

func TestClassifyGrantErrorLiftsBalancesOutOfEnglish(t *testing.T) {
	failure := classifyGrantError(rejected("Insufficient balance (need 5000, have 12)"))
	if failure.Code != "grant_insufficient_balance" {
		t.Fatalf("got code %q", failure.Code)
	}
	if !strings.Contains(failure.Message, "需要 5000") || !strings.Contains(failure.Message, "现有 12") {
		t.Errorf("the numbers were not translated: %q", failure.Message)
	}
}

func TestClassifyGrantErrorKeepsShopFaultsApart(t *testing.T) {
	// "The store is misconfigured" and "NodeLoc is down" are not the provider
	// refusing this transfer, and must not read as if they were: one is fixed on
	// the 设置 page, the other by waiting.
	notConfigured := classifyGrantError(fmt.Errorf("%w：缺少 token", domain.ErrPaymentNotConfigured))
	if notConfigured.Code != "not_configured" || notConfigured.Status != 503 {
		t.Errorf("misconfiguration became %q/%d", notConfigured.Code, notConfigured.Status)
	}
	if !strings.Contains(notConfigured.Message, "缺少 token") {
		t.Errorf("the missing field was lost: %q", notConfigured.Message)
	}
	unreachable := classifyGrantError(fmt.Errorf("%w: dial tcp: connection refused", domain.ErrProviderUnreachable))
	if unreachable.Code != "provider_unreachable" || !unreachable.Retryable {
		t.Errorf("an outage must stay retryable, got %+v", unreachable)
	}
}

func TestGrantRefusesLocallyBeforeAskingTheProvider(t *testing.T) {
	cases := []struct {
		name     string
		input    GrantInput
		wantErr  error
		wantCode string
	}{
		{"zero amount", GrantInput{UserID: 7, Amount: 0, OperatorID: 1}, nil, "grant_amount"},
		{"negative amount", GrantInput{UserID: 7, Amount: -50, OperatorID: 1}, nil, "grant_amount"},
		{"over the ceiling", GrantInput{UserID: 7, Amount: maxGrantAmount + 1, OperatorID: 1}, nil, "grant_amount"},
		{"unknown recipient", GrantInput{UserID: 404, Amount: 10, OperatorID: 1}, nil, "grant_recipient_missing"},
		// A banned account is a different fix than a foreign order, so it gets its
		// own refusal instead of the shared 「这个订单不属于当前账号」.
		{"suspended recipient", GrantInput{UserID: 9, Amount: 10, OperatorID: 1}, nil, "grant_recipient_inactive"},
		{"no NodeLoc account", GrantInput{UserID: 8, Amount: 10, OperatorID: 1}, ErrGrantRecipientUnknown, ""},
		{"operator without a name", GrantInput{UserID: 7, Amount: 10, OperatorID: 0}, ErrInvalidInput, ""},
		{"inactive operator", GrantInput{UserID: 7, Amount: 10, OperatorID: 2}, ErrForbidden, ""},
	}
	for _, tc := range cases {
		provider := &grantProvider{}
		svc, ledger, events := grantService(provider, boundUsers())
		transfer, err := svc.Grant(context.Background(), tc.input)
		if err == nil {
			t.Fatalf("%s: refused nothing", tc.name)
		}
		if tc.wantErr != nil && !errors.Is(err, tc.wantErr) {
			t.Errorf("%s: got %v, want %v", tc.name, err, tc.wantErr)
		}
		if tc.wantCode != "" {
			var failure *GrantFailure
			if !errors.As(err, &failure) || failure.Code != tc.wantCode {
				t.Errorf("%s: got %#v, want code %s", tc.name, err, tc.wantCode)
			}
		}
		if transfer != nil {
			t.Errorf("%s: a refused transfer must not come back as one that happened", tc.name)
		}
		if len(provider.requests) != 0 {
			t.Errorf("%s: NodeLoc was asked anyway", tc.name)
		}
		if len(ledger.created) != 0 || len(events.events) != 0 {
			t.Errorf("%s: a request that never left was booked as if it had", tc.name)
		}
	}

	// The amount copy is the only place a shop owner learns the ceiling, so the
	// number has to be in it.
	provider := &grantProvider{}
	svc, _, _ := grantService(provider, boundUsers())
	_, err := svc.Grant(context.Background(), GrantInput{UserID: 7, Amount: maxGrantAmount + 1, OperatorID: 1})
	var failure *GrantFailure
	if !errors.As(err, &failure) || !strings.Contains(failure.Message, strconv.Itoa(maxGrantAmount)) {
		t.Errorf("the ceiling is not in %v", err)
	}

	// The banned-account refusal has to name the thing to fix, not reuse the
	// buyer-facing "this order is not yours" sentence.
	_, err = svc.Grant(context.Background(), GrantInput{UserID: 9, Amount: 10, OperatorID: 1})
	if !errors.As(err, &failure) || !strings.Contains(failure.Message, "禁用") {
		t.Errorf("the suspended copy does not say what to unban: %v", err)
	}

	// So does the one for an account that is simply gone.
	_, err = svc.Grant(context.Background(), GrantInput{UserID: 404, Amount: 10, OperatorID: 1})
	if !errors.As(err, &failure) || !strings.Contains(failure.Message, "没有找到这个账号") {
		t.Errorf("the missing-account copy does not say the account is gone: %v", err)
	}
}

func TestGrantBooksWhatNodeLocDid(t *testing.T) {
	ctx := context.Background()
	provider := &grantProvider{result: &contract.TransferResult{TransactionID: "TR701", Status: domain.StatusSucceeded, Raw: []byte(`{"success":true}`)}}
	svc, ledger, events := grantService(provider, boundUsers())

	transfer, err := svc.Grant(ctx, GrantInput{UserID: 7, Amount: 30, Note: "补差", OperatorID: 1})
	if err != nil {
		t.Fatalf("a completed transfer was reported as a failure: %v", err)
	}
	if len(provider.requests) != 1 {
		t.Fatalf("NodeLoc was asked %d times", len(provider.requests))
	}
	request := provider.requests[0]
	if request.ToUserID != "4242" || request.ToUsername != "buyer_one_nl" || request.Amount != 30 {
		t.Errorf("wrong recipient addressed: %+v", request)
	}
	if !strings.HasPrefix(request.OrderID, "NLG") || len(request.OrderID) < 19 {
		t.Errorf("reference %q is not the shop's timestamped, collision-free shape", request.OrderID)
	}
	if transfer.Status != domain.StatusSucceeded || transfer.ProviderTransactionID == nil || *transfer.ProviderTransactionID != "TR701" {
		t.Errorf("ledger row reads %+v", transfer)
	}
	if transfer.OperatorName != "owner" || transfer.Username != "buyer_one" {
		t.Errorf("the ledger does not name both sides: %+v", transfer)
	}
	if transfer.Note != "补差" {
		t.Errorf("note %q lost", transfer.Note)
	}
	if len(ledger.created) != 1 {
		t.Fatalf("got %d ledger rows", len(ledger.created))
	}
	if len(events.events) != 1 {
		t.Fatalf("got %d buyer notifications, want the transfer announced once", len(events.events))
	}
	event := events.events[0]
	if event.UserID != 7 || event.Type != "transfer" {
		t.Errorf("notification addressed wrong: %+v", event)
	}
	if !strings.Contains(event.Content, "30 NL") || !strings.Contains(event.Content, "补差") {
		t.Errorf("the buyer is not told what arrived: %+v", event)
	}
}

func TestGrantBooksTheRefusalToo(t *testing.T) {
	provider := &grantProvider{err: rejected("Insufficient balance (need 5000, have 12)")}
	svc, ledger, events := grantService(provider, boundUsers())

	_, err := svc.Grant(context.Background(), GrantInput{UserID: 7, Amount: 500, OperatorID: 1})
	var failure *GrantFailure
	if !errors.As(err, &failure) || failure.Code != "grant_insufficient_balance" {
		t.Fatalf("got %v", err)
	}
	if len(ledger.created) != 1 {
		t.Fatalf("a refused transfer left no trace: %d rows", len(ledger.created))
	}
	row := ledger.created[0]
	if row.Status != domain.StatusFailed {
		t.Errorf("refusal booked as %q", row.Status)
	}
	// The ledger and the screen must not be able to disagree about what happened.
	if row.Detail != failure.Message {
		t.Errorf("ledger says %q, the operator was told %q", row.Detail, failure.Message)
	}
	if len(events.events) != 0 {
		t.Error("the buyer was told about money that never arrived")
	}
}

func TestGrantKeepsALedgerWriteFromUndoesTheProvider(t *testing.T) {
	// NodeLoc already moved the points. A ledger that will not take the row must
	// not answer 「转账失败」, or the shop sends a second one.
	provider := &grantProvider{result: &contract.TransferResult{TransactionID: "TR702", Status: domain.StatusSucceeded}}
	svc, ledger, events := grantService(provider, boundUsers())
	ledger.err = errors.New("database is closed")

	transfer, err := svc.Grant(context.Background(), GrantInput{UserID: 7, Amount: 10, OperatorID: 1})
	if err != nil {
		t.Fatalf("a transfer NodeLoc completed came back as a failure: %v", err)
	}
	if transfer.Status != domain.StatusSucceeded {
		t.Errorf("transfer reads %q", transfer.Status)
	}
	if len(events.events) != 1 {
		t.Error("the buyer should still hear about money that really moved")
	}
}

func TestGrantTreatsAnUnfinishedAnswerAsNoMoneyMoved(t *testing.T) {
	cases := []struct {
		name       string
		result     *contract.TransferResult
		wantPhrase string
	}{
		{"no body at all", nil, "没有给出转账结果"},
		{"status says pending", &contract.TransferResult{Status: "pending"}, "未完成"},
		{"empty status", &contract.TransferResult{}, "状态 unknown"},
	}
	for _, tc := range cases {
		provider := &grantProvider{result: tc.result}
		svc, ledger, events := grantService(provider, boundUsers())
		_, err := svc.Grant(context.Background(), GrantInput{UserID: 7, Amount: 10, OperatorID: 1})
		var failure *GrantFailure
		if !errors.As(err, &failure) || failure.Code != "grant_unanswered" {
			t.Fatalf("%s: got %v", tc.name, err)
		}
		if !strings.Contains(failure.Message, tc.wantPhrase) && tc.name != "no body at all" {
			t.Errorf("%s: %q lacks %q", tc.name, failure.Message, tc.wantPhrase)
		}
		if !strings.Contains(failure.Message, "不要") {
			t.Errorf("%s: an unanswered transfer must warn against sending a second one: %q", tc.name, failure.Message)
		}
		if len(ledger.created) != 1 || ledger.created[0].Status != domain.StatusFailed {
			t.Errorf("%s: nothing was booked for the owner to check later", tc.name)
		}
		if len(events.events) != 0 {
			t.Errorf("%s: the buyer was promised money that is not on its way", tc.name)
		}
	}
}

func TestGrantRefDoesNotRepeatItself(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		ref := newGrantRef()
		if seen[ref] {
			t.Fatalf("two transfers built the same reference %q — NodeLoc would refuse the second", ref)
		}
		seen[ref] = true
	}
}

func TestClassifyCarriesGrantFailuresToTheTransport(t *testing.T) {
	failure := Classify(&GrantFailure{Code: "grant_disabled", Status: 409, Message: "转账功能未开启"})
	if failure.Code != "grant_disabled" || failure.Status != 409 || failure.Message != "转账功能未开启" {
		t.Errorf("a refused transfer lost its own wording: %+v", failure)
	}
}
