// Package budget integrates with the external Budget & Finance REST service.
package budget

import "fmt"

// Allocation describes the operating budget allocation returned for the transit department.
type Allocation struct {
	DepartmentCode string  `json:"department_code"`
	Allocated      float64 `json:"allocated"`
	Spent          float64 `json:"spent"`
	Remaining      float64 `json:"remaining"`
	Currency       string  `json:"currency"`
}

// ExpenseInput contains fields submitted when recording a transit operating expense.
type ExpenseInput struct {
	DepartmentCode string  `json:"department_code,omitempty"`
	Amount         float64 `json:"amount"`
	Description    string  `json:"description"`
}

// ExpenseRecord represents an approved operating expense returned by Budget & Finance.
type ExpenseRecord struct {
	ID             string   `json:"id,omitempty"`
	DepartmentCode string   `json:"department_code"`
	Amount         float64  `json:"amount"`
	Description    string   `json:"description"`
	Status         string   `json:"status,omitempty"`
	Remaining      *float64 `json:"remaining,omitempty"`
	CreatedAt      string   `json:"created_at,omitempty"`
}

// RevenueInput contains fields submitted when reporting transit fare revenue.
type RevenueInput struct {
	Source         string  `json:"source,omitempty"`
	DepartmentCode string  `json:"department_code,omitempty"`
	Amount         float64 `json:"amount"`
	Description    string  `json:"description"`
}

// RevenueRecord represents a recorded revenue entry returned by Budget & Finance.
type RevenueRecord struct {
	ID             string  `json:"id,omitempty"`
	Source         string  `json:"source"`
	DepartmentCode string  `json:"department_code"`
	Amount         float64 `json:"amount"`
	Description    string  `json:"description"`
	CreatedAt      string  `json:"created_at,omitempty"`
}

// FundingRequestInput contains fields submitted when requesting additional department funding.
type FundingRequestInput struct {
	DepartmentCode string  `json:"department_code,omitempty"`
	Amount         float64 `json:"amount"`
	Reason         string  `json:"reason"`
	Urgent         bool    `json:"urgent"`
}

// FundingRequestRecord represents a submitted funding request returned by Budget & Finance.
type FundingRequestRecord struct {
	ID             string  `json:"id,omitempty"`
	DepartmentCode string  `json:"department_code"`
	Amount         float64 `json:"amount"`
	Reason         string  `json:"reason"`
	Urgent         bool    `json:"urgent"`
	Status         string  `json:"status,omitempty"`
	CreatedAt      string  `json:"created_at,omitempty"`
}

// InvalidInput reports invalid request arguments supplied by the caller.
type InvalidInput struct {
	Message string
}

func (e *InvalidInput) Error() string {
	return e.Message
}

// RejectedError reports a 400/422/409 rejection returned by Budget & Finance (e.g. over budget).
type RejectedError struct {
	StatusCode int
	Message    string
	TraceID    string
}

func (e *RejectedError) Error() string {
	if e.TraceID != "" {
		return fmt.Sprintf("%s (traceId: %s)", e.Message, e.TraceID)
	}
	return e.Message
}

// UpstreamUnavailableError reports a non-retriable or connection failure when calling Budget & Finance.
type UpstreamUnavailableError struct {
	Message string
}

func (e *UpstreamUnavailableError) Error() string {
	return e.Message
}

// UpstreamTimeoutError reports a timeout when waiting for Budget & Finance.
type UpstreamTimeoutError struct {
	Message string
}

func (e *UpstreamTimeoutError) Error() string {
	return e.Message
}
