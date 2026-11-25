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
		if r.Header.Get("Authorization") != "" {
			t.Errorf("unexpected Authorization header: %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/budget/v1/allocations/TRANSIT":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"departmentCode": "TRANSIT",
				"departmentName": "Public Transport",
				"allocated":      4000000,
				"spent":          0,
				"remaining":      4000000,
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
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"approved":        true,
				"message":         "Expense approved",
				"remainingBudget": 3998749.25,
				"expense": map[string]any{
					"id":             "11111111-1111-1111-1111-111111111111",
					"departmentCode": "TRANSIT",
					"amount":         1250.75,
					"description":    "Diesel fuel",
					"createdAtUtc":   "2026-11-25T06:30:00Z",
				},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/budget/v1/revenue":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode revenue request: %v", err)
			}
			if body["source"] != "FARES" || body["departmentCode"] != "TRANSIT" {
				t.Fatalf("unexpected revenue body: %+v", body)
			}
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"approved": true,
				"message":  "Revenue recorded",
				"revenue": map[string]any{
					"id":             "22222222-2222-2222-2222-222222222222",
					"source":         "FARES",
					"departmentCode": "TRANSIT",
					"amount":         4320.00,
					"description":    "Daily bus farebox collection",
					"createdAtUtc":   "2026-11-25T06:31:00Z",
				},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/budget/v1/funding-requests":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode funding request: %v", err)
			}
			if body["departmentCode"] != "TRANSIT" || body["urgent"] != true {
				t.Fatalf("unexpected funding request body: %+v", body)
			}
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":             "33333333-3333-3333-3333-333333333333",
				"departmentCode": "TRANSIT",
				"amount":         50000,
				"reason":         "Winter fleet maintenance",
				"urgent":         true,
				"status":         0,
				"createdAtUtc":   "2026-11-25T06:32:00Z",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient(config.BudgetConfig{
		ServiceURL:     server.URL,
		DepartmentCode: "TRANSIT",
	}, server.Client())

	alloc, err := client.GetAllocation(context.Background())
	if err != nil {
		t.Fatalf("GetAllocation failed: %v", err)
	}
	if alloc.DepartmentCode != "TRANSIT" || alloc.Allocated != 4000000 || alloc.Spent != 0 || alloc.Remaining != 4000000 || alloc.Currency != "CAD" {
		t.Fatalf("unexpected allocation: %+v", alloc)
	}

	exp, err := client.SubmitExpense(context.Background(), ExpenseInput{
		Amount:      1250.75,
		Description: "Diesel fuel",
	})
	if err != nil {
		t.Fatalf("SubmitExpense failed: %v", err)
	}
	if exp.ID != "11111111-1111-1111-1111-111111111111" || exp.DepartmentCode != "TRANSIT" || exp.Status != "approved" || exp.Remaining == nil || *exp.Remaining != 3998749.25 || exp.CreatedAt != "2026-11-25T06:30:00Z" {
		t.Fatalf("unexpected expense record: %+v", exp)
	}

	rev, err := client.SubmitRevenue(context.Background(), RevenueInput{
		Amount:      4320.00,
		Description: "Daily bus farebox collection",
	})
	if err != nil {
		t.Fatalf("SubmitRevenue failed: %v", err)
	}
	if rev.ID != "22222222-2222-2222-2222-222222222222" || rev.Source != "FARES" || rev.DepartmentCode != "TRANSIT" || rev.CreatedAt != "2026-11-25T06:31:00Z" {
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
	if fr.ID != "33333333-3333-3333-3333-333333333333" || !fr.Urgent || fr.Status != "pending" || fr.CreatedAt != "2026-11-25T06:32:00Z" {
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

	t.Run("maps 200 approved=false envelopes to RejectedError", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			switch r.URL.Path {
			case "/api/budget/v1/expenses":
				_ = json.NewEncoder(w).Encode(map[string]any{
					"approved": false,
					"message":  "Over budget",
				})
			case "/api/budget/v1/revenue":
				_ = json.NewEncoder(w).Encode(map[string]any{
					"approved": false,
					"message":  "Invalid source",
				})
			default:
				http.NotFound(w, r)
			}
		}))
		defer server.Close()

		client := NewClient(config.BudgetConfig{ServiceURL: server.URL, DepartmentCode: "TRANSIT"}, server.Client())

		_, err := client.SubmitExpense(context.Background(), ExpenseInput{Amount: 5000000, Description: "Fleet"})
		var expRejected *RejectedError
		if !errors.As(err, &expRejected) || !strings.Contains(expRejected.Error(), "Over budget") {
			t.Fatalf("expected RejectedError with 'Over budget', got %v", err)
		}

		_, err = client.SubmitRevenue(context.Background(), RevenueInput{Amount: 100, Description: "Test"})
		var revRejected *RejectedError
		if !errors.As(err, &revRejected) || !strings.Contains(revRejected.Error(), "Invalid source") {
			t.Fatalf("expected RejectedError with 'Invalid source', got %v", err)
		}
	})

	t.Run("unmarshals fundingRequestStatus integer enums, strings, and null", func(t *testing.T) {
		for _, tc := range []struct {
			raw  string
			want fundingRequestStatus
		}{
			{`0`, "pending"},
			{`1`, "approved"},
			{`2`, "rejected"},
			{`"Approved"`, "approved"},
			{`null`, ""},
		} {
			var got fundingRequestStatus
			if err := json.Unmarshal([]byte(tc.raw), &got); err != nil {
				t.Fatalf("unmarshal %s: %v", tc.raw, err)
			}
			if got != tc.want {
				t.Fatalf("unmarshal %s = %q, want %q", tc.raw, got, tc.want)
			}
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
