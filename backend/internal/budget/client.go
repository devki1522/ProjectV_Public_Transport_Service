package budget

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/config"
)

const (
	defaultRequestTimeout = 10 * time.Second
	maxResponseBodyBytes  = 1 << 20
	defaultRevenueSource  = "FARES"
)

var errRedirectsDisabled = errors.New("redirects are not allowed for Budget & Finance API calls")

// Client communicates over HTTP REST with the external Budget & Finance service.
type Client struct {
	baseURL     string
	defaultDept string
	client      *http.Client
}

// NewClient creates a Budget & Finance REST client from configuration.
func NewClient(cfg config.BudgetConfig, httpClient *http.Client) *Client {
	safeClient := &http.Client{Timeout: defaultRequestTimeout}
	if httpClient != nil {
		copied := *httpClient
		if copied.Timeout <= 0 {
			copied.Timeout = defaultRequestTimeout
		}
		safeClient = &copied
	}
	safeClient.CheckRedirect = func(*http.Request, []*http.Request) error {
		return errRedirectsDisabled
	}
	defaultDept := strings.ToUpper(strings.TrimSpace(cfg.DepartmentCode))
	if defaultDept == "" {
		defaultDept = config.DefaultBudgetDepartment
	}
	return &Client{
		baseURL:     strings.TrimRight(strings.TrimSpace(cfg.ServiceURL), "/"),
		defaultDept: defaultDept,
		client:      safeClient,
	}
}

type upstreamAllocation struct {
	DepartmentCode string  `json:"departmentCode"`
	Allocated      float64 `json:"allocated"`
	Spent          float64 `json:"spent"`
	Remaining      float64 `json:"remaining"`
	Currency       string  `json:"currency"`
}

type upstreamExpenseRequest struct {
	DepartmentCode string  `json:"departmentCode"`
	Amount         float64 `json:"amount"`
	Description    string  `json:"description"`
}

type upstreamExpense struct {
	ID             string  `json:"id"`
	DepartmentCode string  `json:"departmentCode"`
	Amount         float64 `json:"amount"`
	Description    string  `json:"description"`
	CreatedAtUtc   string  `json:"createdAtUtc"`
	CreatedAt      string  `json:"createdAt"`
}

type upstreamExpenseResponse struct {
	Approved        *bool            `json:"approved"`
	Message         string           `json:"message"`
	RemainingBudget *float64         `json:"remainingBudget"`
	Expense         *upstreamExpense `json:"expense"`
	ID              string           `json:"id"`
	DepartmentCode  string           `json:"departmentCode"`
	Amount          float64          `json:"amount"`
	Description     string           `json:"description"`
	Status          string           `json:"status"`
	Remaining       *float64         `json:"remaining"`
	CreatedAt       string           `json:"createdAt"`
}

type upstreamRevenueRequest struct {
	Source         string  `json:"source"`
	DepartmentCode string  `json:"departmentCode"`
	Amount         float64 `json:"amount"`
	Description    string  `json:"description"`
}

type upstreamRevenue struct {
	ID             string  `json:"id"`
	Source         string  `json:"source"`
	DepartmentCode string  `json:"departmentCode"`
	Amount         float64 `json:"amount"`
	Description    string  `json:"description"`
	CreatedAtUtc   string  `json:"createdAtUtc"`
	CreatedAt      string  `json:"createdAt"`
}

type upstreamRevenueResponse struct {
	Approved       *bool            `json:"approved"`
	Message        string           `json:"message"`
	Revenue        *upstreamRevenue `json:"revenue"`
	ID             string           `json:"id"`
	Source         string           `json:"source"`
	DepartmentCode string           `json:"departmentCode"`
	Amount         float64          `json:"amount"`
	Description    string           `json:"description"`
	CreatedAt      string           `json:"createdAt"`
}

type upstreamFundingRequest struct {
	DepartmentCode string  `json:"departmentCode"`
	Amount         float64 `json:"amount"`
	Reason         string  `json:"reason"`
	Urgent         bool    `json:"urgent"`
}

type fundingRequestStatus string

func (s *fundingRequestStatus) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if bytes.Equal(trimmed, []byte("null")) {
		*s = ""
		return nil
	}
	var str string
	if err := json.Unmarshal(trimmed, &str); err == nil {
		*s = fundingRequestStatus(strings.ToLower(strings.TrimSpace(str)))
		return nil
	}
	var code int
	if err := json.Unmarshal(trimmed, &code); err == nil {
		switch code {
		case 0:
			*s = "pending"
		case 1:
			*s = "approved"
		case 2:
			*s = "rejected"
		default:
			*s = fundingRequestStatus(fmt.Sprintf("status_%d", code))
		}
		return nil
	}
	return fmt.Errorf("invalid funding request status: %s", string(trimmed))
}

type upstreamFundingResponse struct {
	ID             string               `json:"id"`
	DepartmentCode string               `json:"departmentCode"`
	Amount         float64              `json:"amount"`
	Reason         string               `json:"reason"`
	Urgent         bool                 `json:"urgent"`
	Status         fundingRequestStatus `json:"status"`
	CreatedAtUtc   string               `json:"createdAtUtc"`
	CreatedAt      string               `json:"createdAt"`
}

type upstreamError struct {
	Error   string `json:"error"`
	Message string `json:"message"`
	TraceID string `json:"traceId"`
}

// GetAllocation retrieves the transit operating budget allocation from GET /api/budget/v1/allocations/{dept}.
func (c *Client) GetAllocation(ctx context.Context) (Allocation, error) {
	if c.baseURL == "" {
		return Allocation{}, &UpstreamUnavailableError{Message: "budget service URL is not configured"}
	}

	var raw upstreamAllocation
	path := "/api/budget/v1/allocations/" + url.PathEscape(c.defaultDept)
	if err := c.doJSON(ctx, http.MethodGet, path, nil, http.StatusOK, &raw); err != nil {
		return Allocation{}, err
	}

	dept := strings.ToUpper(strings.TrimSpace(raw.DepartmentCode))
	if dept == "" {
		dept = c.defaultDept
	}
	currency := strings.ToUpper(strings.TrimSpace(raw.Currency))
	if currency == "" {
		currency = "CAD"
	}
	return Allocation{
		DepartmentCode: dept,
		Allocated:      raw.Allocated,
		Spent:          raw.Spent,
		Remaining:      raw.Remaining,
		Currency:       currency,
	}, nil
}

// SubmitExpense sends a transit operating expense to POST /api/budget/v1/expenses.
func (c *Client) SubmitExpense(ctx context.Context, input ExpenseInput) (ExpenseRecord, error) {
	dept, err := c.normalizeDepartment(input.DepartmentCode)
	if err != nil {
		return ExpenseRecord{}, err
	}
	if err := validatePositiveAmount("amount", input.Amount); err != nil {
		return ExpenseRecord{}, err
	}
	desc, err := validateText("description", input.Description, 250)
	if err != nil {
		return ExpenseRecord{}, err
	}
	if c.baseURL == "" {
		return ExpenseRecord{}, &UpstreamUnavailableError{Message: "budget service URL is not configured"}
	}

	reqBody := upstreamExpenseRequest{
		DepartmentCode: dept,
		Amount:         input.Amount,
		Description:    desc,
	}
	var raw upstreamExpenseResponse
	if err := c.doJSON(ctx, http.MethodPost, "/api/budget/v1/expenses", reqBody, http.StatusCreated, &raw); err != nil {
		return ExpenseRecord{}, err
	}
	if raw.Approved != nil && !*raw.Approved {
		return ExpenseRecord{}, &RejectedError{
			StatusCode: http.StatusBadRequest,
			Message:    fallbackString(raw.Message, "expense was rejected by budget service"),
		}
	}

	id := raw.ID
	respDept := raw.DepartmentCode
	amount := raw.Amount
	respDesc := raw.Description
	createdAt := raw.CreatedAt
	if raw.Expense != nil {
		id = fallbackString(raw.Expense.ID, id)
		respDept = fallbackString(raw.Expense.DepartmentCode, respDept)
		amount = fallbackAmount(raw.Expense.Amount, amount)
		respDesc = fallbackString(raw.Expense.Description, respDesc)
		createdAt = fallbackString(raw.Expense.CreatedAtUtc, fallbackString(raw.Expense.CreatedAt, createdAt))
	}
	remaining := raw.RemainingBudget
	if remaining == nil {
		remaining = raw.Remaining
	}
	status := fallbackString(raw.Status, "approved")
	if raw.Approved != nil && *raw.Approved {
		status = "approved"
	}

	return ExpenseRecord{
		ID:             id,
		DepartmentCode: fallbackString(respDept, dept),
		Amount:         fallbackAmount(amount, input.Amount),
		Description:    fallbackString(respDesc, desc),
		Status:         status,
		Remaining:      remaining,
		CreatedAt:      createdAt,
	}, nil
}

// SubmitRevenue reports transit fare revenue to POST /api/budget/v1/revenue.
func (c *Client) SubmitRevenue(ctx context.Context, input RevenueInput) (RevenueRecord, error) {
	dept, err := c.normalizeDepartment(input.DepartmentCode)
	if err != nil {
		return RevenueRecord{}, err
	}
	source := strings.ToUpper(strings.TrimSpace(input.Source))
	if source == "" {
		source = defaultRevenueSource
	}
	if utf8.RuneCountInString(source) > 30 {
		return RevenueRecord{}, &InvalidInput{Message: "source must be 30 characters or fewer"}
	}
	if err := validatePositiveAmount("amount", input.Amount); err != nil {
		return RevenueRecord{}, err
	}
	desc, err := validateText("description", input.Description, 250)
	if err != nil {
		return RevenueRecord{}, err
	}
	if c.baseURL == "" {
		return RevenueRecord{}, &UpstreamUnavailableError{Message: "budget service URL is not configured"}
	}

	reqBody := upstreamRevenueRequest{
		Source:         source,
		DepartmentCode: dept,
		Amount:         input.Amount,
		Description:    desc,
	}
	var raw upstreamRevenueResponse
	if err := c.doJSON(ctx, http.MethodPost, "/api/budget/v1/revenue", reqBody, http.StatusCreated, &raw); err != nil {
		return RevenueRecord{}, err
	}
	if raw.Approved != nil && !*raw.Approved {
		return RevenueRecord{}, &RejectedError{
			StatusCode: http.StatusBadRequest,
			Message:    fallbackString(raw.Message, "revenue submission was rejected by budget service"),
		}
	}

	id := raw.ID
	respSource := raw.Source
	respDept := raw.DepartmentCode
	amount := raw.Amount
	respDesc := raw.Description
	createdAt := raw.CreatedAt
	if raw.Revenue != nil {
		id = fallbackString(raw.Revenue.ID, id)
		respSource = fallbackString(raw.Revenue.Source, respSource)
		respDept = fallbackString(raw.Revenue.DepartmentCode, respDept)
		amount = fallbackAmount(raw.Revenue.Amount, amount)
		respDesc = fallbackString(raw.Revenue.Description, respDesc)
		createdAt = fallbackString(raw.Revenue.CreatedAtUtc, fallbackString(raw.Revenue.CreatedAt, createdAt))
	}

	return RevenueRecord{
		ID:             id,
		Source:         fallbackString(respSource, source),
		DepartmentCode: fallbackString(respDept, dept),
		Amount:         fallbackAmount(amount, input.Amount),
		Description:    fallbackString(respDesc, desc),
		CreatedAt:      createdAt,
	}, nil
}

// SubmitFundingRequest requests additional funding via POST /api/budget/v1/funding-requests.
func (c *Client) SubmitFundingRequest(ctx context.Context, input FundingRequestInput) (FundingRequestRecord, error) {
	dept, err := c.normalizeDepartment(input.DepartmentCode)
	if err != nil {
		return FundingRequestRecord{}, err
	}
	if err := validatePositiveAmount("amount", input.Amount); err != nil {
		return FundingRequestRecord{}, err
	}
	reason, err := validateText("reason", input.Reason, 500)
	if err != nil {
		return FundingRequestRecord{}, err
	}
	if c.baseURL == "" {
		return FundingRequestRecord{}, &UpstreamUnavailableError{Message: "budget service URL is not configured"}
	}

	reqBody := upstreamFundingRequest{
		DepartmentCode: dept,
		Amount:         input.Amount,
		Reason:         reason,
		Urgent:         input.Urgent,
	}
	var raw upstreamFundingResponse
	if err := c.doJSON(ctx, http.MethodPost, "/api/budget/v1/funding-requests", reqBody, http.StatusCreated, &raw); err != nil {
		return FundingRequestRecord{}, err
	}

	return FundingRequestRecord{
		ID:             raw.ID,
		DepartmentCode: fallbackString(raw.DepartmentCode, dept),
		Amount:         fallbackAmount(raw.Amount, input.Amount),
		Reason:         fallbackString(raw.Reason, reason),
		Urgent:         input.Urgent || raw.Urgent,
		Status:         fallbackString(string(raw.Status), "pending"),
		CreatedAt:      fallbackString(raw.CreatedAtUtc, raw.CreatedAt),
	}, nil
}

func (c *Client) normalizeDepartment(dept string) (string, error) {
	cleaned := strings.ToUpper(strings.TrimSpace(dept))
	if cleaned == "" {
		return c.defaultDept, nil
	}
	if utf8.RuneCountInString(cleaned) > 20 {
		return "", &InvalidInput{Message: "department_code must be 20 characters or fewer"}
	}
	return cleaned, nil
}

func validatePositiveAmount(field string, amount float64) error {
	if math.IsNaN(amount) || math.IsInf(amount, 0) || amount <= 0 {
		return &InvalidInput{Message: fmt.Sprintf("%s must be greater than 0", field)}
	}
	return nil
}

func validateText(field, value string, maxRunes int) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", &InvalidInput{Message: fmt.Sprintf("%s is required", field)}
	}
	if utf8.RuneCountInString(trimmed) > maxRunes {
		return "", &InvalidInput{Message: fmt.Sprintf("%s must be %d characters or fewer", field, maxRunes)}
	}
	return trimmed, nil
}

func (c *Client) doJSON(ctx context.Context, method, path string, reqBody any, expectedStatus int, out any) error {
	var bodyReader io.Reader
	if reqBody != nil {
		encoded, err := json.Marshal(reqBody)
		if err != nil {
			return fmt.Errorf("encode budget request: %w", err)
		}
		bodyReader = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return &UpstreamUnavailableError{Message: "budget request could not be constructed"}
	}
	req.Header.Set("Accept", "application/json")
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.client.Do(req)
	if err != nil {
		var netErr net.Error
		if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &netErr) && netErr.Timeout()) {
			return &UpstreamTimeoutError{Message: "budget service request timed out"}
		}
		return &UpstreamUnavailableError{Message: "budget service is unavailable"}
	}
	defer resp.Body.Close()

	limited := io.LimitReader(resp.Body, maxResponseBodyBytes+1)
	respBytes, err := io.ReadAll(limited)
	if err != nil {
		return &UpstreamUnavailableError{Message: "failed to read budget service response"}
	}
	if len(respBytes) > maxResponseBodyBytes {
		return &UpstreamUnavailableError{Message: "budget service response exceeded size limit"}
	}

	if resp.StatusCode == expectedStatus || (expectedStatus == http.StatusCreated && resp.StatusCode == http.StatusOK) {
		if len(bytes.TrimSpace(respBytes)) == 0 {
			return nil
		}
		if err := json.Unmarshal(respBytes, out); err != nil {
			return &UpstreamUnavailableError{Message: "budget service returned invalid JSON"}
		}
		return nil
	}

	return mapUpstreamError(resp.StatusCode, respBytes)
}

func mapUpstreamError(statusCode int, body []byte) error {
	var parsed upstreamError
	_ = json.Unmarshal(body, &parsed)
	msg := strings.TrimSpace(parsed.Message)
	if msg == "" {
		msg = strings.TrimSpace(parsed.Error)
	}

	switch statusCode {
	case http.StatusBadRequest, http.StatusConflict, http.StatusUnprocessableEntity:
		if msg == "" {
			msg = fmt.Sprintf("budget request rejected with status %d", statusCode)
		}
		return &RejectedError{
			StatusCode: statusCode,
			Message:    msg,
			TraceID:    strings.TrimSpace(parsed.TraceID),
		}
	case http.StatusGatewayTimeout:
		return &UpstreamTimeoutError{Message: "budget service timed out"}
	default:
		if statusCode >= 500 || statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden || statusCode == http.StatusNotFound {
			return &UpstreamUnavailableError{Message: "budget service is unavailable"}
		}
		return &UpstreamUnavailableError{Message: fmt.Sprintf("budget service returned status %d", statusCode)}
	}
}

func fallbackString(primary, fallback string) string {
	if trimmed := strings.TrimSpace(primary); trimmed != "" {
		return trimmed
	}
	return fallback
}

func fallbackAmount(primary, fallback float64) float64 {
	if primary > 0 {
		return primary
	}
	return fallback
}
