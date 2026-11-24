package staffhr

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

func TestClientOverviewAndAddEmployee(t *testing.T) {
	now := func() time.Time { return time.Date(2026, 11, 19, 18, 0, 0, 0, time.UTC) }

	var capturedAuth string
	var capturedCreate map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/departments/headcount":
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"code": "EMS", "name": "Emergency Services", "active": 7, "onDuty": 2, "vacancies": 2},
				{"code": "PTR", "name": "Public Transport", "active": 3, "onDuty": 1, "vacancies": 1},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/staff":
			if got := r.URL.Query().Get("Department"); got != "PTR" {
				t.Fatalf("Department query = %q, want PTR", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{
						"staffNumber":      "STF-0000013",
						"displayName":      "Greta Pikeworth",
						"firstName":        "Greta",
						"lastName":         "Pikeworth",
						"email":            "greta.pikeworth@cmes.example",
						"phone":            "555-0112",
						"employmentStatus": "Active",
						"employmentType":   "FullTime",
						"homeDepartment":   map[string]any{"code": "PTR", "name": "Public Transport"},
						"currentRole":      "Light Rail Operator",
						"currentService":   "Light Rail Operations",
						"credentialState":  "Valid",
					},
				},
				"page":     1,
				"pageSize": 50,
				"total":    4,
			})
		case r.Method == http.MethodPost && r.URL.Path == "/staff":
			if err := json.NewDecoder(r.Body).Decode(&capturedCreate); err != nil {
				t.Fatal(err)
			}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"staffNumber":      "STF-0000099",
				"displayName":      "Ada Lovelace",
				"firstName":        "Ada",
				"lastName":         "Lovelace",
				"preferredName":    nil,
				"email":            "ada.lovelace@cmes.example",
				"phone":            "555-0199",
				"dateOfBirth":      "1990-05-01",
				"citizenId":        nil,
				"hireDate":         "2026-11-19",
				"employmentStatus": "Active",
				"employmentType":   "FullTime",
				"homeDepartment":   map[string]any{"code": "PTR", "name": "Public Transport"},
				"credentialState":  nil,
				"assignments":      []any{},
				"credentials":      []any{},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := NewClientWithClock(config.StaffHRConfig{
		ServiceURL:     server.URL,
		DepartmentCode: "PTR",
		BearerToken:    "dev-token",
	}, server.Client(), now)

	overview, err := client.Overview(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if capturedAuth != "Bearer dev-token" {
		t.Fatalf("Authorization = %q, want Bearer dev-token", capturedAuth)
	}
	if overview.DepartmentCode != "PTR" || overview.DepartmentName != "Public Transport" || overview.ActiveCount != 3 || overview.OnDutyCount != 1 || overview.VacanciesCount != 1 || overview.TotalEmployees != 4 || overview.CitywideActive != 10 {
		t.Fatalf("unexpected overview: %+v", overview)
	}
	if len(overview.Employees) != 1 || overview.Employees[0].StaffNumber != "STF-0000013" {
		t.Fatalf("unexpected employees: %+v", overview.Employees)
	}

	phone := "555-0199"
	created, err := client.AddEmployee(context.Background(), AddEmployeeInput{
		FirstName:      "Ada",
		LastName:       "Lovelace",
		Email:          "ada.lovelace@cmes.example",
		DateOfBirth:    "1990-05-01",
		HireDate:       "2026-11-19",
		Phone:          &phone,
		EmploymentType: "FullTime",
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.StaffNumber != "STF-0000099" || created.HomeDepartmentCode != "PTR" {
		t.Fatalf("unexpected created employee: %+v", created)
	}
	if capturedCreate["firstName"] != "Ada" || capturedCreate["homeDepartmentCode"] != "PTR" || capturedCreate["employmentStatus"] != "Active" {
		t.Fatalf("unexpected camelCase request payload: %+v", capturedCreate)
	}
}

func TestClientErrorMappingsAndValidation(t *testing.T) {
	now := func() time.Time { return time.Date(2026, 11, 19, 18, 0, 0, 0, time.UTC) }

	t.Run("returns ErrNotConfigured when ServiceURL is empty", func(t *testing.T) {
		client := NewClientWithClock(config.StaffHRConfig{DepartmentCode: "PTR"}, nil, now)
		if _, err := client.Overview(context.Background(), ""); !errors.Is(err, ErrNotConfigured) {
			t.Fatalf("got %v, want ErrNotConfigured", err)
		}
	})

	t.Run("maps 409 conflict, 422 problem details, and 503 unavailable", func(t *testing.T) {
		var statusToReturn int
		var bodyToReturn string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/problem+json")
			w.WriteHeader(statusToReturn)
			_, _ = w.Write([]byte(bodyToReturn))
		}))
		defer server.Close()

		client := NewClientWithClock(config.StaffHRConfig{ServiceURL: server.URL, DepartmentCode: "PTR"}, server.Client(), now)
		valid := AddEmployeeInput{
			FirstName:      "Sam",
			LastName:       "Rivera",
			Email:          "sam@cmes.example",
			DateOfBirth:    "1995-03-10",
			HireDate:       "2026-11-19",
			EmploymentType: "PartTime",
		}

		statusToReturn = http.StatusConflict
		bodyToReturn = `{"title":"Conflict","detail":"Work email already exists."}`
		_, err := client.AddEmployee(context.Background(), valid)
		if !errors.Is(err, ErrConflict) || !strings.Contains(err.Error(), "Work email already exists") {
			t.Fatalf("409 mapping failed: %v", err)
		}

		statusToReturn = http.StatusUnprocessableEntity
		bodyToReturn = `{"title":"Validation failed","errors":{"email":["Invalid domain"]}}`
		_, err = client.AddEmployee(context.Background(), valid)
		var invalid *InvalidInput
		if !errors.As(err, &invalid) || !strings.Contains(err.Error(), "Invalid domain") {
			t.Fatalf("422 mapping failed: %v", err)
		}

		statusToReturn = http.StatusServiceUnavailable
		bodyToReturn = `{}`
		_, err = client.Overview(context.Background(), "PTR")
		if !errors.Is(err, ErrUpstreamUnavailable) {
			t.Fatalf("503 mapping failed: %v", err)
		}
	})

	t.Run("validates input before network call", func(t *testing.T) {
		client := NewClientWithClock(config.StaffHRConfig{ServiceURL: "https://hr.example.com", DepartmentCode: "PTR"}, nil, now)
		badCases := []AddEmployeeInput{
			{LastName: "X", Email: "a@b.com", DateOfBirth: "1990-01-01", HireDate: "2026-01-01", EmploymentType: "FullTime"},
			{FirstName: "X", LastName: "Y", Email: "not-an-email", DateOfBirth: "1990-01-01", HireDate: "2026-01-01", EmploymentType: "FullTime"},
			{FirstName: "X", LastName: "Y", Email: "a@b.com", DateOfBirth: "2026-11-20", HireDate: "2026-11-21", EmploymentType: "FullTime"},
			{FirstName: "X", LastName: "Y", Email: "a@b.com", DateOfBirth: "2015-01-01", HireDate: "2026-11-19", EmploymentType: "FullTime"},
			{FirstName: "X", LastName: "Y", Email: "a@b.com", DateOfBirth: "1990-01-01", HireDate: "2026-11-19", EmploymentType: "Freelance"},
		}
		for _, tc := range badCases {
			_, err := client.AddEmployee(context.Background(), tc)
			var invalid *InvalidInput
			if !errors.As(err, &invalid) {
				t.Fatalf("expected InvalidInput for %+v, got %v", tc, err)
			}
		}
	})
}
