package layout

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	authAdapter "github.com/bkielbasa/go-ecommerce/backend/auth/adapter"
	authDomain "github.com/bkielbasa/go-ecommerce/backend/auth/domain"
	checkoutDomain "github.com/bkielbasa/go-ecommerce/backend/checkout/domain"
	checkoutQuery "github.com/bkielbasa/go-ecommerce/backend/checkout/query"
	fulfillmentApp "github.com/bkielbasa/go-ecommerce/backend/fulfillment/app"
	fulfillmentDomain "github.com/bkielbasa/go-ecommerce/backend/fulfillment/domain"
	"github.com/bkielbasa/go-ecommerce/backend/internal/fx"
	promodomain "github.com/bkielbasa/go-ecommerce/backend/promo/domain"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
)

type mockStatusCheckoutCmds struct {
	placeFn                func(ctx context.Context, sessID, customerID, cardNumber string, shipTo checkoutDomain.Address, shipMethod checkoutDomain.ShippingMethod, payMethod checkoutDomain.PaymentMethod, discount promodomain.Discount) (checkoutDomain.Order, error)
	cancelFn               func(ctx context.Context, orderID, customerID string) error
	adminCancelFn          func(ctx context.Context, orderID string) error
	markPaidFn             func(ctx context.Context, orderID string) error
	markPaymentFailedFn    func(ctx context.Context, orderID string, reason string) error
	listShippingMethodsFn  func(ctx context.Context) ([]checkoutDomain.ShippingMethod, error)
	findShippingMethodFn   func(ctx context.Context, code string) (checkoutDomain.ShippingMethod, error)
	updateShippingMethodFn func(ctx context.Context, code string, enabled bool, label string, cost int64, carrier string) error
	updateTrackingFn       func(ctx context.Context, orderID, carrier, trackingCode string) error
}

func (m *mockStatusCheckoutCmds) Place(ctx context.Context, sessID, customerID, cardNumber string, shipTo checkoutDomain.Address, shipMethod checkoutDomain.ShippingMethod, payMethod checkoutDomain.PaymentMethod, discount promodomain.Discount) (checkoutDomain.Order, error) {
	if m.placeFn != nil {
		return m.placeFn(ctx, sessID, customerID, cardNumber, shipTo, shipMethod, payMethod, discount)
	}
	return checkoutDomain.Order{}, nil
}

func (m *mockStatusCheckoutCmds) Cancel(ctx context.Context, orderID, customerID string) error {
	if m.cancelFn != nil {
		return m.cancelFn(ctx, orderID, customerID)
	}
	return nil
}

func (m *mockStatusCheckoutCmds) AdminCancel(ctx context.Context, orderID string) error {
	if m.adminCancelFn != nil {
		return m.adminCancelFn(ctx, orderID)
	}
	return nil
}

func (m *mockStatusCheckoutCmds) MarkPaid(ctx context.Context, orderID string) error {
	if m.markPaidFn != nil {
		return m.markPaidFn(ctx, orderID)
	}
	return nil
}

func (m *mockStatusCheckoutCmds) MarkPaymentFailed(ctx context.Context, orderID string, reason string) error {
	if m.markPaymentFailedFn != nil {
		return m.markPaymentFailedFn(ctx, orderID, reason)
	}
	return nil
}

func (m *mockStatusCheckoutCmds) ListShippingMethods(ctx context.Context) ([]checkoutDomain.ShippingMethod, error) {
	if m.listShippingMethodsFn != nil {
		return m.listShippingMethodsFn(ctx)
	}
	return nil, nil
}

func (m *mockStatusCheckoutCmds) FindShippingMethod(ctx context.Context, code string) (checkoutDomain.ShippingMethod, error) {
	if m.findShippingMethodFn != nil {
		return m.findShippingMethodFn(ctx, code)
	}
	return checkoutDomain.ShippingMethod{}, nil
}

func (m *mockStatusCheckoutCmds) UpdateShippingMethod(ctx context.Context, code string, enabled bool, label string, cost int64, carrier string) error {
	if m.updateShippingMethodFn != nil {
		return m.updateShippingMethodFn(ctx, code, enabled, label, cost, carrier)
	}
	return nil
}

func (m *mockStatusCheckoutCmds) UpdateTracking(ctx context.Context, orderID, carrier, trackingCode string) error {
	if m.updateTrackingFn != nil {
		return m.updateTrackingFn(ctx, orderID, carrier, trackingCode)
	}
	return nil
}

type mockStatusCheckoutQry struct {
	findFn func(ctx context.Context, id string) (checkoutQuery.OrderView, error)
}

func (m *mockStatusCheckoutQry) Find(ctx context.Context, id string) (checkoutQuery.OrderView, error) {
	if m.findFn != nil {
		return m.findFn(ctx, id)
	}
	return checkoutQuery.OrderView{}, checkoutDomain.ErrOrderNotFound
}

func (m *mockStatusCheckoutQry) ListByCustomer(ctx context.Context, customerID string) ([]checkoutQuery.OrderSummary, error) {
	return nil, nil
}

func (m *mockStatusCheckoutQry) ListAll(ctx context.Context) ([]checkoutQuery.OrderSummary, error) {
	return nil, nil
}

func (m *mockStatusCheckoutQry) HasPurchasedProduct(ctx context.Context, customerID, productID string) (bool, error) {
	return false, nil
}

func (m *mockStatusCheckoutQry) TodaysSales(ctx context.Context) (map[string]checkoutQuery.DailySalesRow, error) {
	return nil, nil
}

type mockStatusFulfillmentSrv struct {
	onOrderPaidFn    func(ctx context.Context, orderID string, at time.Time) error
	labelFn          func(ctx context.Context, orderID, carrier, trackingCode string) error
	shipFn           func(ctx context.Context, orderID string) error
	deliverFn        func(ctx context.Context, orderID string) error
	refundFn         func(ctx context.Context, orderID, reason string) error
	byOrderFn        func(ctx context.Context, orderID string) (fulfillmentDomain.Fulfillment, error)
	updateTrackingFn func(ctx context.Context, orderID, carrier, trackingCode string) error
	setStatusFn      func(ctx context.Context, orderID string, target fulfillmentDomain.Status) error
}

func (m *mockStatusFulfillmentSrv) OnOrderPaid(ctx context.Context, orderID string, at time.Time) error {
	if m.onOrderPaidFn != nil {
		return m.onOrderPaidFn(ctx, orderID, at)
	}
	return nil
}

func (m *mockStatusFulfillmentSrv) Label(ctx context.Context, orderID, carrier, trackingCode string) error {
	if m.labelFn != nil {
		return m.labelFn(ctx, orderID, carrier, trackingCode)
	}
	return nil
}

func (m *mockStatusFulfillmentSrv) Ship(ctx context.Context, orderID string) error {
	if m.shipFn != nil {
		return m.shipFn(ctx, orderID)
	}
	return nil
}

func (m *mockStatusFulfillmentSrv) Deliver(ctx context.Context, orderID string) error {
	if m.deliverFn != nil {
		return m.deliverFn(ctx, orderID)
	}
	return nil
}

func (m *mockStatusFulfillmentSrv) Refund(ctx context.Context, orderID, reason string) error {
	if m.refundFn != nil {
		return m.refundFn(ctx, orderID, reason)
	}
	return nil
}

func (m *mockStatusFulfillmentSrv) ByOrder(ctx context.Context, orderID string) (fulfillmentDomain.Fulfillment, error) {
	if m.byOrderFn != nil {
		return m.byOrderFn(ctx, orderID)
	}
	return fulfillmentDomain.Fulfillment{}, fulfillmentApp.ErrNotFound
}

func (m *mockStatusFulfillmentSrv) UpdateTracking(ctx context.Context, orderID, carrier, trackingCode string) error {
	if m.updateTrackingFn != nil {
		return m.updateTrackingFn(ctx, orderID, carrier, trackingCode)
	}
	return nil
}

func (m *mockStatusFulfillmentSrv) SetStatus(ctx context.Context, orderID string, target fulfillmentDomain.Status) error {
	if m.setStatusFn != nil {
		return m.setStatusFn(ctx, orderID, target)
	}
	if _, err := m.ByOrder(ctx, orderID); err != nil {
		return err
	}
	return nil
}

func newTestOrderStatusHandler(checkoutCmds checkoutCommands, checkoutQry checkoutQueries, fulfillmentSrv fulfillmentService) httpHandler {
	logger := logrus.New()
	logger.SetLevel(logrus.PanicLevel)
	rates := fx.New("USD", "USD", "USD:1.0", logger)
	admin := authAdapter.Admin{ID: "admin@example.com", Email: "admin@example.com", Role: "admin"}
	sess := authDomain.NewSession("admin-session-token", "admin@example.com", time.Now().Add(time.Hour))
	adminAuth := fakeAdminAuthService{admin: admin, sess: sess}

	return httpHandler{
		checkoutSrv:    checkoutCmds,
		checkoutQry:    checkoutQry,
		fulfillmentSrv: fulfillmentSrv,
		adminAuthSrv:   adminAuth,
		rates:          rates,
		logger:         logger,
	}
}

func makeTestOrderView(orderID string, status checkoutDomain.Status, carrier, trackingCode string) checkoutQuery.OrderView {
	return checkoutQuery.NewOrderView(
		orderID, "cust-1", status, time.Now(),
		nil, checkoutDomain.Address{}, checkoutDomain.ShippingMethod{}, checkoutDomain.PaymentMethod{},
		1000, 0, 0, 1000, "USD",
		carrier, trackingCode,
		"", 0,
	)
}

func TestAdminUpdateOrderStatus_Unauthorized(t *testing.T) {
	setupTestEnvironment(t)
	handler := newTestOrderStatusHandler(&mockStatusCheckoutCmds{}, &mockStatusCheckoutQry{}, &mockStatusFulfillmentSrv{})

	req := httptest.NewRequest(http.MethodPost, "/admin/orders/ord-1/status", nil)
	rec := httptest.NewRecorder()

	handler.AdminUpdateOrderStatus(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected status 303, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/admin/login" {
		t.Fatalf("expected redirect to /admin/login, got %s", loc)
	}
}

func TestAdminUpdateOrderStatus_UpdateTracking(t *testing.T) {
	setupTestEnvironment(t)

	orderID := "ord-track-1"
	orderView := makeTestOrderView(orderID, checkoutDomain.StatusPaid, "", "")

	var updatedCheckoutCarrier, updatedCheckoutTracking string
	var updatedFulfillmentCarrier, updatedFulfillmentTracking string

	checkoutCmds := &mockStatusCheckoutCmds{
		updateTrackingFn: func(ctx context.Context, oID, carrier, trackingCode string) error {
			if oID != orderID {
				t.Errorf("expected orderID %s, got %s", orderID, oID)
			}
			updatedCheckoutCarrier = carrier
			updatedCheckoutTracking = trackingCode
			return nil
		},
	}

	checkoutQry := &mockStatusCheckoutQry{
		findFn: func(ctx context.Context, id string) (checkoutQuery.OrderView, error) {
			if id == orderID {
				return orderView, nil
			}
			return checkoutQuery.OrderView{}, checkoutDomain.ErrOrderNotFound
		},
	}

	ff := fulfillmentDomain.Rebuild("ff-1", orderID, fulfillmentDomain.StatusScheduled, "", "", time.Now(), time.Time{}, time.Time{}, "", 1)
	fulfillmentSrv := &mockStatusFulfillmentSrv{
		byOrderFn: func(ctx context.Context, oID string) (fulfillmentDomain.Fulfillment, error) {
			if oID == orderID {
				return ff, nil
			}
			return fulfillmentDomain.Fulfillment{}, fulfillmentApp.ErrNotFound
		},
		updateTrackingFn: func(ctx context.Context, oID, carrier, trackingCode string) error {
			if oID != orderID {
				t.Errorf("expected orderID %s, got %s", orderID, oID)
			}
			updatedFulfillmentCarrier = carrier
			updatedFulfillmentTracking = trackingCode
			return nil
		},
	}

	handler := newTestOrderStatusHandler(checkoutCmds, checkoutQry, fulfillmentSrv)

	form := url.Values{
		"payment_status":  {"paid"},
		"delivery_status": {"scheduled"},
		"carrier":         {"DHL Express"},
		"tracking_code":   {"DHL-998877"},
	}

	req := httptest.NewRequest(http.MethodPost, "/admin/orders/"+orderID+"/status", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = mux.SetURLVars(req, map[string]string{"orderID": orderID})
	setAdminSession(t, req)
	rec := httptest.NewRecorder()

	handler.AdminUpdateOrderStatus(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected status 303, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/admin/orders/"+orderID {
		t.Fatalf("expected redirect to /admin/orders/%s, got %s", orderID, loc)
	}

	if updatedCheckoutCarrier != "DHL Express" || updatedCheckoutTracking != "DHL-998877" {
		t.Errorf("checkout tracking not updated: carrier=%q, tracking=%q", updatedCheckoutCarrier, updatedCheckoutTracking)
	}
	if updatedFulfillmentCarrier != "DHL Express" || updatedFulfillmentTracking != "DHL-998877" {
		t.Errorf("fulfillment tracking not updated: carrier=%q, tracking=%q", updatedFulfillmentCarrier, updatedFulfillmentTracking)
	}

	// Verify flash message
	s, _ := store.Get(req, "ecommerce")
	flashes := s.Flashes()
	if len(flashes) == 0 || !strings.Contains(flashes[0].(string), "Order status and tracking updated successfully.") {
		t.Errorf("expected success flash message, got %v", flashes)
	}
}

func TestAdminUpdateOrderStatus_PaymentStatus_Paid(t *testing.T) {
	setupTestEnvironment(t)

	orderID := "ord-pay-1"
	orderView := makeTestOrderView(orderID, checkoutDomain.StatusPending, "", "")

	var markPaidCalled bool
	var onOrderPaidCalled bool

	checkoutCmds := &mockStatusCheckoutCmds{
		markPaidFn: func(ctx context.Context, oID string) error {
			if oID == orderID {
				markPaidCalled = true
			}
			return nil
		},
	}

	checkoutQry := &mockStatusCheckoutQry{
		findFn: func(ctx context.Context, id string) (checkoutQuery.OrderView, error) {
			if id == orderID {
				return orderView, nil
			}
			return checkoutQuery.OrderView{}, checkoutDomain.ErrOrderNotFound
		},
	}

	fulfillmentSrv := &mockStatusFulfillmentSrv{
		byOrderFn: func(ctx context.Context, oID string) (fulfillmentDomain.Fulfillment, error) {
			return fulfillmentDomain.Fulfillment{}, fulfillmentApp.ErrNotFound
		},
		onOrderPaidFn: func(ctx context.Context, oID string, at time.Time) error {
			if oID == orderID {
				onOrderPaidCalled = true
			}
			return nil
		},
	}

	handler := newTestOrderStatusHandler(checkoutCmds, checkoutQry, fulfillmentSrv)

	form := url.Values{
		"payment_status":  {"paid"},
		"delivery_status": {"scheduled"},
	}

	req := httptest.NewRequest(http.MethodPost, "/admin/orders/"+orderID+"/status", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = mux.SetURLVars(req, map[string]string{"orderID": orderID})
	setAdminSession(t, req)
	rec := httptest.NewRecorder()

	handler.AdminUpdateOrderStatus(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected status 303, got %d", rec.Code)
	}

	if !markPaidCalled {
		t.Errorf("expected MarkPaid to be called for %s", orderID)
	}
	if !onOrderPaidCalled {
		t.Errorf("expected OnOrderPaid to be called for %s", orderID)
	}
}

func TestAdminUpdateOrderStatus_PaymentStatus_Failed(t *testing.T) {
	setupTestEnvironment(t)

	orderID := "ord-fail-1"
	orderView := makeTestOrderView(orderID, checkoutDomain.StatusPending, "", "")

	var markPaymentFailedCalled bool
	var failedReason string

	checkoutCmds := &mockStatusCheckoutCmds{
		markPaymentFailedFn: func(ctx context.Context, oID string, reason string) error {
			if oID == orderID {
				markPaymentFailedCalled = true
				failedReason = reason
			}
			return nil
		},
	}

	checkoutQry := &mockStatusCheckoutQry{
		findFn: func(ctx context.Context, id string) (checkoutQuery.OrderView, error) {
			if id == orderID {
				return orderView, nil
			}
			return checkoutQuery.OrderView{}, checkoutDomain.ErrOrderNotFound
		},
	}

	fulfillmentSrv := &mockStatusFulfillmentSrv{}

	handler := newTestOrderStatusHandler(checkoutCmds, checkoutQry, fulfillmentSrv)

	form := url.Values{
		"payment_status": {"failed"},
	}

	req := httptest.NewRequest(http.MethodPost, "/admin/orders/"+orderID+"/status", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = mux.SetURLVars(req, map[string]string{"orderID": orderID})
	setAdminSession(t, req)
	rec := httptest.NewRecorder()

	handler.AdminUpdateOrderStatus(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected status 303, got %d", rec.Code)
	}

	if !markPaymentFailedCalled {
		t.Errorf("expected MarkPaymentFailed to be called for %s", orderID)
	}
	if failedReason != "admin_override" {
		t.Errorf("expected reason 'admin_override', got %q", failedReason)
	}
}

func TestAdminUpdateOrderStatus_PaymentStatus_Cancelled(t *testing.T) {
	setupTestEnvironment(t)

	orderID := "ord-cancel-1"
	orderView := makeTestOrderView(orderID, checkoutDomain.StatusPaid, "", "")

	var adminCancelCalled bool

	checkoutCmds := &mockStatusCheckoutCmds{
		adminCancelFn: func(ctx context.Context, oID string) error {
			if oID == orderID {
				adminCancelCalled = true
			}
			return nil
		},
	}

	checkoutQry := &mockStatusCheckoutQry{
		findFn: func(ctx context.Context, id string) (checkoutQuery.OrderView, error) {
			if id == orderID {
				return orderView, nil
			}
			return checkoutQuery.OrderView{}, checkoutDomain.ErrOrderNotFound
		},
	}

	fulfillmentSrv := &mockStatusFulfillmentSrv{}

	handler := newTestOrderStatusHandler(checkoutCmds, checkoutQry, fulfillmentSrv)

	form := url.Values{
		"payment_status": {"cancelled"},
	}

	req := httptest.NewRequest(http.MethodPost, "/admin/orders/"+orderID+"/status", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = mux.SetURLVars(req, map[string]string{"orderID": orderID})
	setAdminSession(t, req)
	rec := httptest.NewRecorder()

	handler.AdminUpdateOrderStatus(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected status 303, got %d", rec.Code)
	}

	if !adminCancelCalled {
		t.Errorf("expected AdminCancel to be called for %s", orderID)
	}
}

func TestAdminUpdateOrderStatus_DeliveryStatus_ShippedAndDelivered(t *testing.T) {
	setupTestEnvironment(t)

	orderID := "ord-deliv-1"
	orderView := makeTestOrderView(orderID, checkoutDomain.StatusPaid, "", "")

	var setStatusTarget fulfillmentDomain.Status

	checkoutCmds := &mockStatusCheckoutCmds{}
	checkoutQry := &mockStatusCheckoutQry{
		findFn: func(ctx context.Context, id string) (checkoutQuery.OrderView, error) {
			if id == orderID {
				return orderView, nil
			}
			return checkoutQuery.OrderView{}, checkoutDomain.ErrOrderNotFound
		},
	}

	ff := fulfillmentDomain.Rebuild("ff-1", orderID, fulfillmentDomain.StatusScheduled, "", "", time.Now(), time.Time{}, time.Time{}, "", 1)
	fulfillmentSrv := &mockStatusFulfillmentSrv{
		byOrderFn: func(ctx context.Context, oID string) (fulfillmentDomain.Fulfillment, error) {
			if oID == orderID {
				return ff, nil
			}
			return fulfillmentDomain.Fulfillment{}, fulfillmentApp.ErrNotFound
		},
		setStatusFn: func(ctx context.Context, oID string, target fulfillmentDomain.Status) error {
			if oID == orderID {
				setStatusTarget = target
			}
			return nil
		},
	}

	handler := newTestOrderStatusHandler(checkoutCmds, checkoutQry, fulfillmentSrv)

	// Transition to shipped
	form := url.Values{
		"payment_status":  {"paid"},
		"delivery_status": {"shipped"},
	}

	req := httptest.NewRequest(http.MethodPost, "/admin/orders/"+orderID+"/status", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = mux.SetURLVars(req, map[string]string{"orderID": orderID})
	setAdminSession(t, req)
	rec := httptest.NewRecorder()

	handler.AdminUpdateOrderStatus(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected status 303, got %d", rec.Code)
	}

	if setStatusTarget != fulfillmentDomain.StatusShipped {
		t.Errorf("expected setStatus target 'shipped', got %q", setStatusTarget)
	}

	// Now transition to delivered
	ffDeliv := fulfillmentDomain.Rebuild("ff-1", orderID, fulfillmentDomain.StatusShipped, "", "", time.Now(), time.Now(), time.Time{}, "", 2)
	fulfillmentSrv.byOrderFn = func(ctx context.Context, oID string) (fulfillmentDomain.Fulfillment, error) {
		return ffDeliv, nil
	}

	formDeliv := url.Values{
		"payment_status":  {"paid"},
		"delivery_status": {"delivered"},
	}

	reqDeliv := httptest.NewRequest(http.MethodPost, "/admin/orders/"+orderID+"/status", strings.NewReader(formDeliv.Encode()))
	reqDeliv.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqDeliv = mux.SetURLVars(reqDeliv, map[string]string{"orderID": orderID})
	setAdminSession(t, reqDeliv)
	recDeliv := httptest.NewRecorder()

	handler.AdminUpdateOrderStatus(recDeliv, reqDeliv)

	if recDeliv.Code != http.StatusSeeOther {
		t.Fatalf("expected status 303, got %d", recDeliv.Code)
	}

	if setStatusTarget != fulfillmentDomain.StatusDelivered {
		t.Errorf("expected setStatus target 'delivered', got %q", setStatusTarget)
	}
}

func TestAdminOrderDetail_RendersStatusAndTrackingForm(t *testing.T) {
	setupTestEnvironment(t)

	orderID := "ord-render-1"
	orderView := makeTestOrderView(orderID, checkoutDomain.StatusPaid, "FedEx", "FX-12345")

	checkoutCmds := &mockStatusCheckoutCmds{}
	checkoutQry := &mockStatusCheckoutQry{
		findFn: func(ctx context.Context, id string) (checkoutQuery.OrderView, error) {
			if id == orderID {
				return orderView, nil
			}
			return checkoutQuery.OrderView{}, checkoutDomain.ErrOrderNotFound
		},
	}

	ff := fulfillmentDomain.Rebuild("ff-1", orderID, fulfillmentDomain.StatusScheduled, "FedEx", "FX-12345", time.Now(), time.Time{}, time.Time{}, "", 1)
	fulfillmentSrv := &mockStatusFulfillmentSrv{
		byOrderFn: func(ctx context.Context, oID string) (fulfillmentDomain.Fulfillment, error) {
			if oID == orderID {
				return ff, nil
			}
			return fulfillmentDomain.Fulfillment{}, fulfillmentApp.ErrNotFound
		},
	}

	handler := newTestOrderStatusHandler(checkoutCmds, checkoutQry, fulfillmentSrv)

	req := httptest.NewRequest(http.MethodGet, "/admin/orders/"+orderID, nil)
	req = mux.SetURLVars(req, map[string]string{"orderID": orderID})
	setAdminSession(t, req)
	rec := httptest.NewRecorder()

	handler.AdminOrderDetail(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d. Body: %s", rec.Code, rec.Body.String())
	}

	body := rec.Body.String()
	for _, expected := range []string{
		"/admin/orders/" + orderID + "/status",
		"payment_status",
		"delivery_status",
		"carrier",
		"tracking_code",
		"Update Status & Tracking",
		"FedEx",
		"FX-12345",
	} {
		if !strings.Contains(body, expected) {
			t.Errorf("expected body to contain %q, but it didn't", expected)
		}
	}
}

func TestAdminUpdateOrderStatus_RouteRegistered(t *testing.T) {
	setupTestEnvironment(t)

	orderID := "ord-route-1"
	orderView := makeTestOrderView(orderID, checkoutDomain.StatusPaid, "", "")

	var updateTrackingCalled bool
	checkoutCmds := &mockStatusCheckoutCmds{
		updateTrackingFn: func(ctx context.Context, oID, carrier, trackingCode string) error {
			updateTrackingCalled = true
			return nil
		},
	}
	checkoutQry := &mockStatusCheckoutQry{
		findFn: func(ctx context.Context, id string) (checkoutQuery.OrderView, error) {
			if id == orderID {
				return orderView, nil
			}
			return checkoutQuery.OrderView{}, checkoutDomain.ErrOrderNotFound
		},
	}

	handler := newTestOrderStatusHandler(checkoutCmds, checkoutQry, &mockStatusFulfillmentSrv{})
	bc := boundedContext{
		handler: handler,
		logger:  handler.logger,
	}

	router := mux.NewRouter()
	bc.MuxRegister(router)

	req := httptest.NewRequest(http.MethodPost, "/admin/orders/"+orderID+"/status", strings.NewReader("carrier=UPS&tracking_code=1Z999"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	setAdminSession(t, req)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected status 303, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/admin/orders/"+orderID {
		t.Fatalf("expected redirect to /admin/orders/%s, got %s", orderID, loc)
	}
	if !updateTrackingCalled {
		t.Errorf("expected UpdateTracking to be called through router")
	}
}

func TestAdminUpdateOrderStatus_PendingOrder_WithDeliveryStatusFormValue_DoesNot500(t *testing.T) {
	setupTestEnvironment(t)

	orderID := "ord-pending-123"
	orderView := makeTestOrderView(orderID, checkoutDomain.StatusPending, "", "")

	var updateTrackingCalled bool
	checkoutCmds := &mockStatusCheckoutCmds{
		updateTrackingFn: func(ctx context.Context, oID, carrier, trackingCode string) error {
			updateTrackingCalled = true
			if carrier != "DHL" || trackingCode != "TRACK-PENDING" {
				t.Errorf("unexpected carrier/tracking: %s / %s", carrier, trackingCode)
			}
			return nil
		},
	}
	checkoutQry := &mockStatusCheckoutQry{
		findFn: func(ctx context.Context, id string) (checkoutQuery.OrderView, error) {
			if id == orderID {
				return orderView, nil
			}
			return checkoutQuery.OrderView{}, checkoutDomain.ErrOrderNotFound
		},
	}
	fulfillmentSrv := &mockStatusFulfillmentSrv{
		byOrderFn: func(ctx context.Context, oID string) (fulfillmentDomain.Fulfillment, error) {
			return fulfillmentDomain.Fulfillment{}, fulfillmentApp.ErrNotFound
		},
	}

	handler := newTestOrderStatusHandler(checkoutCmds, checkoutQry, fulfillmentSrv)

	form := url.Values{}
	form.Set("payment_status", "pending")
	form.Set("delivery_status", "scheduled")
	form.Set("carrier", "DHL")
	form.Set("tracking_code", "TRACK-PENDING")

	req := httptest.NewRequest(http.MethodPost, "/admin/orders/"+orderID+"/status", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = mux.SetURLVars(req, map[string]string{"orderID": orderID})
	setAdminSession(t, req)

	rec := httptest.NewRecorder()
	handler.AdminUpdateOrderStatus(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected status 303, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	if !updateTrackingCalled {
		t.Errorf("expected UpdateTracking to be called")
	}
}
