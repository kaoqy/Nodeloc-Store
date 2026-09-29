package http

import (
	"encoding/csv"
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	middleware "github.com/kaoqy/Nodeloc-Store/internal/app/httpserver"
	"github.com/kaoqy/Nodeloc-Store/internal/config"
	"github.com/kaoqy/Nodeloc-Store/internal/models"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/payment/application"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/payment/domain"
)

type Handler struct {
	service *application.Service
}

type createPaymentRequest struct {
	OrderNo     string `json:"order_no" binding:"required"`
	Description string `json:"description"`
}

type createOrderRequest struct {
	Slug       string `json:"slug" binding:"required"`
	Quantity   int    `json:"quantity"`
	Contact    string `json:"contact"`
	Note       string `json:"note"`
	CouponCode string `json:"coupon_code"`
}

func NewHandler(service *application.Service) *Handler {
	if service == nil {
		panic("payment: nil service")
	}
	return &Handler{service: service}
}

func (h *Handler) RegisterRoutes(router gin.IRouter, jwtConfig *config.JWTConfig, accounts middleware.AccountReader) {
	payment := router.Group("/api/v1/payment")
	payment.Use(middleware.JWTMiddleware(jwtConfig))
	payment.POST("/orders", h.CreateOrder)
	payment.POST("/create", h.CreatePayment)
	payment.GET("/return", h.Return)
	payment.GET("/orders/:order_no", h.GetOrder)
	payment.POST("/orders/:order_no/reconcile", h.ReconcileOrder)
	payment.GET("/orders", h.ListOrders)

	// Admin order management. Each route names the permission it needs, so 客服
	// can look orders up and 运营 can work them without either holding 设置.
	guard := func(resource, action string) gin.HandlerFunc {
		return middleware.RequirePermission(accounts, resource, action)
	}

	adminOrders := router.Group("/api/v1/admin/orders")
	adminOrders.Use(middleware.JWTMiddleware(jwtConfig))
	adminOrders.GET("", guard("orders", "view"), h.AdminListOrders)
	// A static segment, so it wins over /:order_no for the word "export".
	adminOrders.GET("/export", guard("orders", "view"), h.AdminExportOrders)
	adminOrders.GET("/:order_no", guard("orders", "view"), h.AdminGetOrder)
	adminOrders.POST("/:order_no/cancel", guard("orders", "manage"), h.AdminCancelOrder)
	adminOrders.POST("/:order_no/deliver", guard("orders", "manage"), h.AdminDeliverOrder)
	adminOrders.POST("/:order_no/refund", guard("orders", "manage"), h.AdminRefundOrder)
	adminOrders.POST("/:order_no/fulfill", guard("orders", "manage"), h.AdminFulfillOrder)
	adminOrders.POST("/:order_no/reconcile", guard("orders", "manage"), h.AdminReconcileOrder)

	// A separate group because /orders/:order_no/... already owns that segment.
	adminReconcile := router.Group("/api/v1/admin/reconcile")
	adminReconcile.Use(middleware.JWTMiddleware(jwtConfig))
	adminReconcile.POST("/pending", guard("orders", "manage"), h.AdminReconcilePending)

	// NodeLoc notifies via a browser GET redirect (signature-verified); POST is
	// accepted as well for server-push style integrations.
	router.GET("/api/v1/payment/callback", h.Callback)
	router.POST("/api/v1/payment/callback", h.Callback)
}

func (h *Handler) CreateOrder(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}

	var request createOrderRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	order, err := h.service.CreateOrder(c.Request.Context(), application.CreateOrderInput{
		UserID:     userID,
		Slug:       strings.TrimSpace(request.Slug),
		Quantity:   request.Quantity,
		Contact:    strings.TrimSpace(request.Contact),
		Note:       strings.TrimSpace(request.Note),
		CouponCode: strings.TrimSpace(request.CouponCode),
	})
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"order": order})
}

func (h *Handler) CreatePayment(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}

	var request createPaymentRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	result, err := h.service.CreatePayment(c.Request.Context(), application.CreatePaymentInput{
		UserID:      userID,
		OrderNo:     strings.TrimSpace(request.OrderNo),
		Description: strings.TrimSpace(request.Description),
	})
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, result)
}

func (h *Handler) Callback(c *gin.Context) {
	sets, err := callbackParamSets(c)
	if err != nil || len(sets) == 0 {
		log.Printf("payment callback: unreadable payload from %s: %v", c.ClientIP(), err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid callback payload"})
		return
	}

	result, err := h.service.HandleCallbackSets(c.Request.Context(), sets)
	orderNo := ""
	if result != nil {
		orderNo = result.OrderNo
	}
	code := "ok"
	if err != nil {
		code = callbackErrorCode(err)
		log.Printf("payment callback rejected (%s) from %s: %v", code, c.ClientIP(), err)
	}

	if c.Request.Method == http.MethodGet {
		// Browser redirect flow: land the buyer on the order either way, but say
		// why a payment did not settle so the page can explain it instead of
		// silently showing 待支付.
		if err == nil && orderNo != "" {
			c.Redirect(http.StatusFound, "/orders/"+url.PathEscape(orderNo)+"?pay=ok")
			return
		}
		c.Redirect(http.StatusFound, "/orders?pay="+code)
		return
	}
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

// callbackErrorCode keeps provider detail server-side while still telling the
// buyer which kind of problem to report.
func callbackErrorCode(err error) string {
	switch {
	case errors.Is(err, application.ErrInvalidCallback):
		return "signature"
	case errors.Is(err, application.ErrAmountMismatch):
		return "amount"
	case errors.Is(err, application.ErrPaymentNotComplete):
		return "pending"
	case errors.Is(err, application.ErrPaymentUnsettled), errors.Is(err, application.ErrNoProviderTransaction):
		return "unsettled"
	case errors.Is(err, domain.ErrProviderUnreachable):
		return "unreachable"
	case errors.Is(err, domain.ErrProviderRejected):
		return "rejected"
	case errors.Is(err, domain.ErrOrderNotFound), errors.Is(err, domain.ErrPaymentOrderNotFound):
		return "unknown_order"
	default:
		return "error"
	}
}

func (h *Handler) Return(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}

	orderNo := strings.TrimSpace(c.Query("order_id"))
	if orderNo == "" {
		orderNo = strings.TrimSpace(c.Query("order_no"))
	}
	if orderNo == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "order_id is required"})
		return
	}

	order, err := h.service.GetOrder(c.Request.Context(), userID, orderNo)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"order": order})
}

func (h *Handler) GetOrder(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}

	orderNo := strings.TrimSpace(c.Param("order_no"))
	if orderNo == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "order_no is required"})
		return
	}

	order, err := h.service.GetOrder(c.Request.Context(), userID, orderNo)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"order": order})
}

func (h *Handler) ListOrders(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}

	limit, err := parseNonNegativeInt(c.DefaultQuery("limit", "20"))
	if err != nil || limit == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "limit must be a positive integer"})
		return
	}
	if limit > 100 {
		limit = 100
	}
	offset, err := parseNonNegativeInt(c.DefaultQuery("offset", "0"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "offset must be a non-negative integer"})
		return
	}

	result, err := h.service.ListOrders(c.Request.Context(), userID, limit, offset, c.Query("status"), c.Query("q"))
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

// ReconcileOrder asks NodeLoc what it recorded for the order's payment and
// settles it here when the provider says it went through. Buyers use it after a
// redirect that never reached the store; "still unpaid" comes back as a normal
// answer, not an error, so the storefront can say what to do next.
func (h *Handler) ReconcileOrder(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}
	result, err := h.service.ReconcileOrder(c.Request.Context(), c.Param("order_no"), userID)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

// callbackParamSets returns the payload variants to verify. ParseForm rewrites
// '+' as a space, which breaks the recomputed signature whenever a signed value
// such as paid_at=2026-09-26T18:00:00+08:00 arrives unencoded, so the raw
// query/body is offered as a second candidate decoded without that rewrite.
func callbackParamSets(c *gin.Context) ([]map[string]string, error) {
	contentType := strings.ToLower(c.GetHeader("Content-Type"))
	if strings.Contains(contentType, "application/json") {
		var payload map[string]any
		if err := c.ShouldBindJSON(&payload); err != nil {
			return nil, err
		}
		params := make(map[string]string, len(payload))
		for key, value := range payload {
			switch typed := value.(type) {
			case string:
				params[key] = typed
			case float64:
				params[key] = strconv.FormatFloat(typed, 'f', -1, 64)
			case bool:
				params[key] = strconv.FormatBool(typed)
			}
		}
		return []map[string]string{params}, nil
	}

	raw := c.Request.URL.RawQuery
	if c.Request.Method == http.MethodPost {
		body, err := c.GetRawData()
		if err != nil {
			return nil, err
		}
		raw = string(body)
		c.Request.Body = io.NopCloser(strings.NewReader(raw))
	}
	if err := c.Request.ParseForm(); err != nil {
		return nil, err
	}

	decoded := make(map[string]string, len(c.Request.Form))
	for key, values := range c.Request.Form {
		if len(values) != 0 {
			decoded[key] = values[0]
		}
	}
	if len(decoded) == 0 {
		return nil, errors.New("empty callback payload")
	}
	sets := []map[string]string{decoded}
	if literal := parseFormLiteral(raw); len(literal) > 0 && !sameParams(decoded, literal) {
		sets = append(sets, literal)
	}
	return sets, nil
}

// parseFormLiteral percent-decodes a x-www-form-urlencoded string while leaving
// '+' inside values alone.
func parseFormLiteral(raw string) map[string]string {
	params := map[string]string{}
	for _, pair := range strings.Split(raw, "&") {
		if pair == "" {
			continue
		}
		key, value, found := strings.Cut(pair, "=")
		if !found {
			continue
		}
		decodedKey, err := url.PathUnescape(key)
		if err != nil {
			continue
		}
		decodedValue, err := url.PathUnescape(value)
		if err != nil {
			continue
		}
		params[decodedKey] = decodedValue
	}
	return params
}

func sameParams(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for key, value := range a {
		if b[key] != value {
			return false
		}
	}
	return true
}

func currentUserID(c *gin.Context) (uint, bool) {
	value, exists := c.Get("user_id")
	if !exists {
		return 0, false
	}
	switch id := value.(type) {
	case uint:
		return id, id != 0
	case uint64:
		return uint(id), id != 0
	case int:
		return uint(id), id > 0
	case int64:
		return uint(id), id > 0
	case float64:
		return uint(id), id > 0
	case string:
		parsed, err := strconv.ParseUint(id, 10, 64)
		return uint(parsed), err == nil && parsed != 0
	default:
		return 0, false
	}
}

func parseNonNegativeInt(value string) (int, error) {
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return 0, errors.New("invalid integer")
	}
	return parsed, nil
}

func writeError(c *gin.Context, err error) {
	failure := application.Classify(err)
	if failure.Code == "not_configured" || failure.Code == "internal" {
		// The technical reason names the missing credential or the failing
		// query; it belongs in the log, and on an admin route in the response.
		log.Printf("%s %s: %v", c.Request.Method, c.Request.URL.Path, err)
	}
	body := gin.H{"error": failure.Message, "code": failure.Code, "retryable": failure.Retryable}
	if isAdminRoute(c) && failure.Detail != "" {
		body["error"] = failure.Detail
		body["detail"] = failure.Detail
	}
	c.JSON(failure.Status, body)
}

// isAdminRoute marks the back office, where the shop owner is the reader: the
// storefront gets buyer-safe copy while the admin panel sees NodeLoc's own
// words, which are what make a credential problem fixable.
func isAdminRoute(c *gin.Context) bool {
	return strings.HasPrefix(c.Request.URL.Path, "/api/v1/admin/")
}

// ── Admin Order Handlers ───────────────────────────────────────────

// orderFilter is what both the admin order list and its CSV download read out of
// the query string. Sharing the parse is the point: a download can then only
// ever carry the batch the page was showing.
type orderFilter struct {
	status    string
	search    string
	buyerID   uint
	attention string
}

func (h *Handler) AdminListOrders(c *gin.Context) {
	limit, err := parseNonNegativeInt(c.DefaultQuery("limit", "20"))
	if err != nil || limit == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "limit must be a positive integer"})
		return
	}
	if limit > 100 {
		limit = 100
	}
	offset, err := parseNonNegativeInt(c.DefaultQuery("offset", "0"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "offset must be a non-negative integer"})
		return
	}
	filter, badQuery := readOrderFilter(c)
	if badQuery != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": badQuery})
		return
	}

	result, err := h.service.AdminListOrders(c.Request.Context(), limit, offset, filter.status, filter.search, filter.buyerID, filter.attention)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result.Orders, "total": result.Total, "limit": result.Limit, "offset": result.Offset})
}

func readOrderFilter(c *gin.Context) (orderFilter, string) {
	filter := orderFilter{status: c.Query("status"), search: c.Query("q"), attention: c.Query("attention")}
	buyerID, err := parseNonNegativeInt(c.DefaultQuery("user_id", "0"))
	if err != nil {
		return filter, "user_id must be a non-negative integer"
	}
	filter.buyerID = uint(buyerID)
	return filter, ""
}

// exportOrderColumns name the CSV header. The amounts are labelled in 元 because
// that is the unit the shop prices, displays and charges in: an order for a ¥99
// product reads 99.00 here, on the storefront, and in what NodeLoc was asked to
// collect. A download that rescaled them would not reconcile with anything.
var exportOrderColumns = []string{
	"订单号", "状态", "交付状态", "商品", "数量", "单价(元)", "优惠(元)", "实付(元)", "优惠码",
	"买家ID", "买家用户名", "买家邮箱", "联系方式", "NodeLoc交易号", "卡密数",
	"下单时间", "支付时间", "交付时间",
}

// AdminExportOrders streams the filtered order list as a CSV download for
// bookkeeping and reconciliation. The UTF-8 BOM is not decoration: Excel opens a
// Chinese header row as mojibake without it.
func (h *Handler) AdminExportOrders(c *gin.Context) {
	filter, badQuery := readOrderFilter(c)
	if badQuery != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": badQuery})
		return
	}
	orders, truncated, err := h.service.ExportOrders(c.Request.Context(), filter.status, filter.search, filter.buyerID, filter.attention)
	if err != nil {
		writeError(c, err)
		return
	}
	// The name carries no request input, so nothing from the query string can
	// reach a response header.
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", `attachment; filename="orders-`+time.Now().Format("20060102")+`.csv"`)
	// The page reports these back to the shop owner instead of counting the rows
	// itself: a product name carrying a newline would be miscounted.
	c.Header("X-Export-Rows", strconv.Itoa(len(orders)))
	if truncated {
		c.Header("X-Export-Truncated", "1")
	}
	c.Writer.Write([]byte{0xEF, 0xBB, 0xBF})
	writer := csv.NewWriter(c.Writer)
	_ = writer.Write(exportOrderColumns)
	for _, order := range orders {
		buyerID, username, email := "", "", ""
		if order.User != nil {
			buyerID = strconv.FormatUint(uint64(order.User.ID), 10)
			username = order.User.Username
			if order.User.Email != nil {
				email = *order.User.Email
			}
		}
		transactionID := ""
		if order.TransactionID != nil {
			transactionID = *order.TransactionID
		}
		contact := ""
		if order.CustomerContact != nil {
			contact = *order.CustomerContact
		}
		_ = writer.Write([]string{
			safeCell(order.OrderNo),
			order.Status,
			order.FulfillmentStatus,
			safeCell(productName(order)),
			strconv.Itoa(order.Quantity),
			yuan(order.UnitPrice),
			yuan(order.DiscountAmount),
			yuan(order.TotalAmount),
			safeCell(order.CouponCode),
			buyerID,
			safeCell(username),
			safeCell(email),
			safeCell(contact),
			safeCell(transactionID),
			strconv.Itoa(len(order.Cards)),
			order.CreatedAt.Format(time.RFC3339),
			formatTime(order.PaidAt),
			formatTime(order.DeliveredAt),
		})
	}
	writer.Flush()
	if truncated {
		_, _ = c.Writer.Write([]byte("# 结果已截断，请缩小筛选范围后再导出\n"))
	}
}

// safeCell defuses spreadsheet formulas. A buyer types their own contact
// details, and a cell starting with = + - @ executes when the shop owner opens
// the export in Excel — a download of orders has to stay data, not code.
func safeCell(value string) string {
	if value == "" {
		return value
	}
	switch value[0] {
	case '=', '+', '-', '@', '\t', '\r':
		return "'" + value
	}
	return value
}

func productName(order models.Order) string {
	if order.Product != nil {
		return order.Product.Name
	}
	return ""
}

func formatTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.Format(time.RFC3339)
}

// yuan renders an order amount the way a statement reads it: the shop's own
// number, two decimals, no rescaling.
func yuan(amount int) string {
	return strconv.FormatFloat(float64(amount), 'f', 2, 64)
}

func (h *Handler) AdminGetOrder(c *gin.Context) {
	orderNo := strings.TrimSpace(c.Param("order_no"))
	if orderNo == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "order_no is required"})
		return
	}
	order, err := h.service.AdminGetOrder(c.Request.Context(), orderNo)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": order})
}

func (h *Handler) AdminCancelOrder(c *gin.Context) {
	orderNo := strings.TrimSpace(c.Param("order_no"))
	if orderNo == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "order_no is required"})
		return
	}
	order, err := h.service.AdminCancelOrder(c.Request.Context(), orderNo)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": order})
}

type adminDeliverRequest struct {
	DeliveryContent string `json:"delivery_content" binding:"required"`
}

func (h *Handler) AdminDeliverOrder(c *gin.Context) {
	orderNo := strings.TrimSpace(c.Param("order_no"))
	if orderNo == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "order_no is required"})
		return
	}
	var request adminDeliverRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	order, err := h.service.AdminDeliverOrder(c.Request.Context(), orderNo, request.DeliveryContent)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": order})
}

func (h *Handler) AdminRefundOrder(c *gin.Context) {
	orderNo := strings.TrimSpace(c.Param("order_no"))
	if orderNo == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "order_no is required"})
		return
	}
	order, err := h.service.AdminRefundOrder(c.Request.Context(), orderNo)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": order})
}

// AdminFulfillOrder retries delivery, e.g. after card stock has been restocked
// for an order that was parked in waiting_stock.
func (h *Handler) AdminFulfillOrder(c *gin.Context) {
	orderNo := strings.TrimSpace(c.Param("order_no"))
	if orderNo == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "order_no is required"})
		return
	}
	order, err := h.service.FulfillOrder(c.Request.Context(), orderNo)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": order})
}

// AdminReconcileOrder is the back-office 查单 button: settle an order the
// provider already marks as paid without waiting for the buyer's browser.
func (h *Handler) AdminReconcileOrder(c *gin.Context) {
	result, err := h.service.ReconcileOrder(c.Request.Context(), c.Param("order_no"), 0)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

// AdminReconcilePending runs 查单 over every order the store still calls 待支付
// but which has a NodeLoc transaction id — the cleanup after a batch of lost
// redirects, which no buyer thinks to press the button for.
func (h *Handler) AdminReconcilePending(c *gin.Context) {
	report, err := h.service.AdminReconcilePending(c.Request.Context())
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, report)
}
