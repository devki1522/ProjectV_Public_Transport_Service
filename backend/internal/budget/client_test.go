package budget

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/config"
)

func TestClientBudgetEndpoints(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer budget-secret" {
			t.Errorf("unexpected Authorization header: %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/budget/v1/allocations/transit":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"departmentCode": "TRANSIT",
				"allocated":      250000.00,
				"spent":          90000.50,
				"remaining":      159999.50,
				"currency":       "CAD",
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/budget/v1/expenses":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode expense request: %v", err)
			}
			if body["departmentCode"] != "TRANSIT" || body["description"] != "Diesel fuel" {
				t.Fatalf("unexpected expense body: %+v", body)
			}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":             "exp-101",
				"departmentCode": "TRANSIT",
				"amount":         1250.75,
				"description":    "Diesel fuel",
				"status":         "approved",
				"remaining":      158748.75,
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/budget/v1/revenue":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode revenue request: %v", err)
			}
			if body["source"] != "FARES" || body["departmentCode"] != "TRANSIT" {
				t.Fatalf("unexpected revenue body: %+v", body)
			}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":             "rev-202",
				"source":         "FARES",
				"departmentCode": "TRANSIT",
				"amount":         4320.00,
				"description":    "Daily bus farebox collection",
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/budget/v1/funding-requests":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode funding request: %v", err)
			}
			if body["departmentCode"] != "TRANSIT" || body["urgent"] != true {
				t.Fatalf("unexpected funding request body: %+v", body)
			}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":             "fund-303",
				"departmentCode": "TRANSIT",
				"amount":         50000,
				"reason":         "Winter fleet maintenance",
				"urgent":         true,
				"status":         "pending",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient(config.BudgetConfig{
		ServiceURL:     server.URL,
		DepartmentCode: "TRANSIT",
		BearerToken:    "budget-secret",
	}, server.Client())

	alloc, err := client.GetAllocation(context.Background())
	if err != nil {
		t.Fatalf("GetAllocation failed: %v", err)
	}
	if alloc.DepartmentCode != "TRANSIT" || alloc.Allocated != 250000 || alloc.Remaining != 159999.50 || alloc.Currency != "CAD" {
		t.Fatalf("unexpected allocation: %+v", alloc)
	}

	exp, err := client.SubmitExpense(context.Background(), ExpenseInput{
		Amount:      1250.75,
		Description: "Diesel fuel",
	})
	if err != nil {
		t.Fatalf("SubmitExpense failed: %v", err)
	}
	if exp.ID != "exp-101" || exp.DepartmentCode != "TRANSIT" || exp.Status != "approved" {
		t.Fatalf("unexpected expense record: %+v", exp)
	}

	rev, err := client.SubmitRevenue(context.Background(), RevenueInput{
		Amount:      4320.00,
		Description: "Daily bus farebox collection",
	})
	if err != nil {
		t.Fatalf("SubmitRevenue failed: %v", err)
	}
	if rev.ID != "rev-202" || rev.Source != "FARES" || rev.DepartmentCode != "TRANSIT" {
		t.Fatalf("unexpected revenue record: %+v", rev)
	}

	fr, err := client.SubmitFundingRequest(context.Background(), FundingRequestInput{
		Amount: 50000,
		Reason: "Winter fleet maintenance",
		Urgent: true,
	})
	if err != nil {
		t.Fatalf("SubmitFundingRequest failed: %v", err)
	}
	if fr.ID != "fund-303" || !fr.Urgent || fr.Status != "pending" {
		t.Fatalf("unexpected funding request record: %+v", fr)
	}
}

func TestClientErrorMappingsAndValidation(t *testing.T) {
	t.Run("validates input before calling upstream", func(t *testing.T) {
		client := NewClient(config.BudgetConfig{ServiceURL: "https://budget.example.com", DepartmentCode: "TRANSIT"}, nil)
		if _, err := client.SubmitExpense(context.Background(), ExpenseInput{Amount: 0, Description: "Fuel"}); err == nil {
			t.Fatal("expected InvalidInput for zero amount")
		}
		if _, err := client.SubmitExpense(context.Background(), ExpenseInput{Amount: 100, Description: "   "}); err == nil {
			t.Fatal("expected InvalidInput for blank description")
		}
		if _, err := client.SubmitRevenue(context.Background(), RevenueInput{Amount: -5, Description: "Fares"}); err == nil {
			t.Fatal("expected InvalidInput for negative revenue")
		}
		if _, err := client.SubmitFundingRequest(context.Background(), FundingRequestInput{Amount: 100, Reason: ""}); err == nil {
			t.Fatal("expected InvalidInput for blank reason")
		}
	})

	t.Run("maps 400 over-budget error with traceId and 503 unavailable", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if r.URL.Path == "/api/budget/v1/expenses" {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"error":   "OVER_BUDGET",
					"message": "Expense exceeds remaining transit budget",
					"traceId": "trace-abc-123",
				})
				return
			}
			w.WriteHeader(http.StatusServiceUnavailable)
		}))
		defer server.Close()

		client := NewClient(config.BudgetConfig{ServiceURL: server.URL, DepartmentCode: "TRANSIT"}, server.Client())

		_, err := client.SubmitExpense(context.Background(), ExpenseInput{Amount: 999999, Description: "New bus"})
		var rejected *RejectedError
		if !errors.As(err, &rejected) {
			t.Fatalf("expected RejectedError, got %v", err)
		}
		if rejected.StatusCode != http.StatusBadRequest || !strings.Contains(rejected.Error(), "Expense exceeds remaining transit budget") || rejected.TraceID != "trace-abc-123" {
			t.Fatalf("unexpected RejectedError: %+v", rejected)
		}

		_, err = client.GetAllocation(context.Background())
		var unavail *UpstreamUnavailableError
		if !errors.As(err, &unavail) {
			t.Fatalf("expected UpstreamUnavailableError, got %v", err)
		}
	})

	t.Run("maps timeout to UpstreamTimeoutError", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(80 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		httpClient := server.Client()
		httpClient.Timeout = 20 * time.Millisecond
		client := NewClient(config.BudgetConfig{ServiceURL: server.URL, DepartmentCode: "TRANSIT"}, httpClient)

		_, err := client.GetAllocation(context.Background())
		var timeoutErr *UpstreamTimeoutError
		if !errors.As(err, &timeoutErr) {
			t.Fatalf("expected UpstreamTimeoutError, got %v", err)
		}
	})
}
