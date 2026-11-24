package staffhr

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/config"
)

const (
	defaultRequestTimeout = 10 * time.Second
	maxResponseBodyBytes  = 1 << 20
)

var (
	errRedirectsDisabled = errors.New("redirects are not allowed for Staff/HR API calls")

	allowedEmploymentTypes = map[string]bool{
		"FullTime": true,
		"PartTime": true,
		"Contract": true,
		"Seasonal": true,
	}
	allowedOnboardingStatuses = map[string]bool{
		"Active":    true,
		"Probation": true,
	}
)

// Client communicates exclusively over HTTP REST with the external CMES Staff/HR service.
type Client struct {
	baseURL     string
	defaultDept string
	bearerToken string
	client      *http.Client
	now         func() time.Time
}

// NewClient creates a Staff/HR REST client from configuration.
func NewClient(cfg config.StaffHRConfig, httpClient *http.Client) *Client {
	return NewClientWithClock(cfg, httpClient, time.Now)
}

// NewClientWithClock creates a Staff/HR REST client with an injectable clock for deterministic validation tests.
func NewClientWithClock(cfg config.StaffHRConfig, httpClient *http.Client, now func() time.Time) *Client {
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
	if now == nil {
		now = time.Now
	}
	defaultDept := strings.ToUpper(strings.TrimSpace(cfg.DepartmentCode))
	if defaultDept == "" {
		defaultDept = config.DefaultStaffHRDepartment
	}
	return &Client{
		baseURL:     strings.TrimRight(strings.TrimSpace(cfg.ServiceURL), "/"),
		defaultDept: defaultDept,
		bearerToken: strings.TrimSpace(cfg.BearerToken),
		client:      safeClient,
		now:         now,
	}
}

type upstreamDepartmentHeadcount struct {
	Code      string `json:"code"`
	Name      string `json:"name"`
	Active    int    `json:"active"`
	OnDuty    int    `json:"onDuty"`
	Vacancies int    `json:"vacancies"`
}

type upstreamDeptRef struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

type upstreamStaffSummary struct {
	StaffNumber      string          `json:"staffNumber"`
	DisplayName      string          `json:"displayName"`
	FirstName        string          `json:"firstName"`
	LastName         string          `json:"lastName"`
	Email            string          `json:"email"`
	Phone            *string         `json:"phone"`
	EmploymentStatus string          `json:"employmentStatus"`
	EmploymentType   string          `json:"employmentType"`
	HomeDepartment   upstreamDeptRef `json:"homeDepartment"`
	CurrentRole      *string         `json:"currentRole"`
	CurrentService   *string         `json:"currentService"`
	CredentialState  *string         `json:"credentialState"`
}

type upstreamPagedStaff struct {
	Items    []upstreamStaffSummary `json:"items"`
	Page     int                    `json:"page"`
	PageSize int                    `json:"pageSize"`
	Total    int                    `json:"total"`
}

type upstreamCreateStaffRequest struct {
	FirstName          string  `json:"firstName"`
	LastName           string  `json:"lastName"`
	PreferredName      *string `json:"preferredName,omitempty"`
	Email              string  `json:"email"`
	DateOfBirth        string  `json:"dateOfBirth"`
	Phone              *string `json:"phone,omitempty"`
	CitizenID          *string `json:"citizenId,omitempty"`
	HireDate           string  `json:"hireDate"`
	EmploymentType     string  `json:"employmentType"`
	EmploymentStatus   string  `json:"employmentStatus"`
	HomeDepartmentCode string  `json:"homeDepartmentCode"`
}

type upstreamAssignment struct {
	RoleTitle   string  `json:"roleTitle"`
	ServiceName string  `json:"serviceName"`
	EndDate     *string `json:"endDate"`
}

type upstreamStaffProfile struct {
	StaffNumber      string               `json:"staffNumber"`
	DisplayName      string               `json:"displayName"`
	FirstName        string               `json:"firstName"`
	LastName         string               `json:"lastName"`
	Email            string               `json:"email"`
	Phone            *string              `json:"phone"`
	EmploymentStatus string               `json:"employmentStatus"`
	EmploymentType   string               `json:"employmentType"`
	HomeDepartment   upstreamDeptRef      `json:"homeDepartment"`
	CredentialState  *string              `json:"credentialState"`
	Assignments      []upstreamAssignment `json:"assignments"`
}

type upstreamProblemDetails struct {
	Title  string              `json:"title"`
	Detail string              `json:"detail"`
	Errors map[string][]string `json:"errors"`
}

// Overview queries department headcounts and staff directory rows for departmentCode (defaulting to PTR).
func (c *Client) Overview(ctx context.Context, departmentCode string) (StaffOverview, error) {
	if c == nil || c.baseURL == "" {
		return StaffOverview{}, ErrNotConfigured
	}
	dept, err := normalizeDepartmentCode(departmentCode, c.defaultDept)
	if err != nil {
		return StaffOverview{}, err
	}

	var headcounts []upstreamDepartmentHeadcount
	if err := c.doJSON(ctx, http.MethodGet, c.baseURL+"/departments/headcount", nil, &headcounts); err != nil {
		return StaffOverview{}, err
	}

	staffQuery := url.Values{}
	staffQuery.Set("Department", dept)
	staffQuery.Set("Page", "1")
	staffQuery.Set("PageSize", fmt.Sprintf("%d", MaxEmployeePageSize))

	var paged upstreamPagedStaff
	if err := c.doJSON(ctx, http.MethodGet, c.baseURL+"/staff?"+staffQuery.Encode(), nil, &paged); err != nil {
		return StaffOverview{}, err
	}

	departments := make([]DepartmentHeadcount, 0, len(headcounts))
	overview := StaffOverview{
		DepartmentCode: dept,
		DepartmentName: dept,
		TotalEmployees: paged.Total,
		Departments:    departments,
		Employees:      make([]StaffMember, 0, len(paged.Items)),
	}

	for _, item := range headcounts {
		dc := DepartmentHeadcount{
			Code:      item.Code,
			Name:      item.Name,
			Active:    item.Active,
			OnDuty:    item.OnDuty,
			Vacancies: item.Vacancies,
		}
		overview.Departments = append(overview.Departments, dc)
		overview.CitywideActive += item.Active
		if strings.EqualFold(item.Code, dept) {
			overview.DepartmentName = item.Name
			overview.ActiveCount = item.Active
			overview.OnDutyCount = item.OnDuty
			overview.VacanciesCount = item.Vacancies
		}
	}

	for _, item := range paged.Items {
		overview.Employees = append(overview.Employees, StaffMember{
			StaffNumber:        item.StaffNumber,
			DisplayName:        item.DisplayName,
			FirstName:          item.FirstName,
			LastName:           item.LastName,
			Email:              item.Email,
			Phone:              item.Phone,
			EmploymentStatus:   item.EmploymentStatus,
			EmploymentType:     item.EmploymentType,
			HomeDepartmentCode: item.HomeDepartment.Code,
			HomeDepartmentName: item.HomeDepartment.Name,
			CurrentRole:        item.CurrentRole,
			CurrentService:     item.CurrentService,
			CredentialState:    item.CredentialState,
		})
	}

	return overview, nil
}

// AddEmployee validates employee onboarding details and creates a new staff record in CMES Staff/HR.
func (c *Client) AddEmployee(ctx context.Context, input AddEmployeeInput) (StaffMember, error) {
	if c == nil || c.baseURL == "" {
		return StaffMember{}, ErrNotConfigured
	}

	reqBody, err := c.validateAndBuildCreateRequest(input)
	if err != nil {
		return StaffMember{}, err
	}

	var profile upstreamStaffProfile
	if err := c.doJSON(ctx, http.MethodPost, c.baseURL+"/staff", reqBody, &profile); err != nil {
		return StaffMember{}, err
	}

	var role, service *string
	for _, a := range profile.Assignments {
		if a.EndDate == nil {
			if a.RoleTitle != "" {
				r := a.RoleTitle
				role = &r
			}
			if a.ServiceName != "" {
				s := a.ServiceName
				service = &s
			}
			break
		}
	}

	return StaffMember{
		StaffNumber:        profile.StaffNumber,
		DisplayName:        profile.DisplayName,
		FirstName:          profile.FirstName,
		LastName:           profile.LastName,
		Email:              profile.Email,
		Phone:              profile.Phone,
		EmploymentStatus:   profile.EmploymentStatus,
		EmploymentType:     profile.EmploymentType,
		HomeDepartmentCode: profile.HomeDepartment.Code,
		HomeDepartmentName: profile.HomeDepartment.Name,
		CurrentRole:        role,
		CurrentService:     service,
		CredentialState:    profile.CredentialState,
	}, nil
}

func (c *Client) validateAndBuildCreateRequest(input AddEmployeeInput) (upstreamCreateStaffRequest, error) {
	firstName, err := validateRequiredText("first_name", input.FirstName, 100)
	if err != nil {
		return upstreamCreateStaffRequest{}, err
	}
	lastName, err := validateRequiredText("last_name", input.LastName, 100)
	if err != nil {
		return upstreamCreateStaffRequest{}, err
	}
	email, err := validateRequiredText("email", input.Email, 254)
	if err != nil {
		return upstreamCreateStaffRequest{}, err
	}
	if !strings.Contains(email, "@") || strings.HasPrefix(email, "@") || strings.HasSuffix(email, "@") {
		return upstreamCreateStaffRequest{}, &InvalidInput{Message: "email must be a valid email address"}
	}

	dobStr := strings.TrimSpace(input.DateOfBirth)
	dob, err := time.Parse("2006-01-02", dobStr)
	if err != nil {
		return upstreamCreateStaffRequest{}, &InvalidInput{Message: "date_of_birth must use YYYY-MM-DD format"}
	}
	nowUTC := c.now().UTC()
	today := time.Date(nowUTC.Year(), nowUTC.Month(), nowUTC.Day(), 0, 0, 0, 0, time.UTC)
	if !dob.Before(today) {
		return upstreamCreateStaffRequest{}, &InvalidInput{Message: "date_of_birth must be in the past"}
	}

	hireStr := strings.TrimSpace(input.HireDate)
	hireDate, err := time.Parse("2006-01-02", hireStr)
	if err != nil {
		return upstreamCreateStaffRequest{}, &InvalidInput{Message: "hire_date must use YYYY-MM-DD format"}
	}
	if hireDate.Before(dob.AddDate(16, 0, 0)) {
		return upstreamCreateStaffRequest{}, &InvalidInput{Message: "hire_date must be at least 16 years after date_of_birth"}
	}

	empType := strings.TrimSpace(input.EmploymentType)
	if !allowedEmploymentTypes[empType] {
		return upstreamCreateStaffRequest{}, &InvalidInput{Message: "employment_type must be FullTime, PartTime, Contract, or Seasonal"}
	}

	empStatus := strings.TrimSpace(input.EmploymentStatus)
	if empStatus == "" {
		empStatus = "Active"
	}
	if !allowedOnboardingStatuses[empStatus] {
		return upstreamCreateStaffRequest{}, &InvalidInput{Message: "employment_status must be Active or Probation"}
	}

	dept, err := normalizeDepartmentCode(input.HomeDepartmentCode, c.defaultDept)
	if err != nil {
		return upstreamCreateStaffRequest{}, err
	}

	prefName, err := normalizeOptionalText("preferred_name", input.PreferredName, 100)
	if err != nil {
		return upstreamCreateStaffRequest{}, err
	}
	phone, err := normalizeOptionalText("phone", input.Phone, 30)
	if err != nil {
		return upstreamCreateStaffRequest{}, err
	}
	citizenID, err := normalizeOptionalText("citizen_id", input.CitizenID, 30)
	if err != nil {
		return upstreamCreateStaffRequest{}, err
	}

	return upstreamCreateStaffRequest{
		FirstName:          firstName,
		LastName:           lastName,
		PreferredName:      prefName,
		Email:              email,
		DateOfBirth:        dobStr,
		Phone:              phone,
		CitizenID:          citizenID,
		HireDate:           hireStr,
		EmploymentType:     empType,
		EmploymentStatus:   empStatus,
		HomeDepartmentCode: dept,
	}, nil
}

func (c *Client) doJSON(ctx context.Context, method, targetURL string, reqBody any, dst any) error {
	var bodyReader io.Reader
	if reqBody != nil {
		raw, err := json.Marshal(reqBody)
		if err != nil {
			return fmt.Errorf("encode Staff/HR request: %w", err)
		}
		bodyReader = bytes.NewReader(raw)
	}

	req, err := http.NewRequestWithContext(ctx, method, targetURL, bodyReader)
	if err != nil {
		return fmt.Errorf("build Staff/HR request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.bearerToken)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return ctx.Err()
		}
		if errors.Is(err, context.DeadlineExceeded) || isTimeoutError(err) {
			return ErrUpstreamTimeout
		}
		return ErrUpstreamUnavailable
	}
	defer resp.Body.Close()

	rawBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodyBytes+1))
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || isTimeoutError(err) {
			return ErrUpstreamTimeout
		}
		return ErrUpstreamUnavailable
	}
	if len(rawBody) > maxResponseBodyBytes {
		return ErrUpstreamUnavailable
	}

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if dst == nil || len(bytes.TrimSpace(rawBody)) == 0 {
			return nil
		}
		if err := json.Unmarshal(rawBody, dst); err != nil {
			return ErrUpstreamUnavailable
		}
		return nil
	}

	return mapUpstreamError(resp.StatusCode, rawBody)
}

func mapUpstreamError(status int, rawBody []byte) error {
	var problem upstreamProblemDetails
	_ = json.Unmarshal(rawBody, &problem)
	msg := formatProblemMessage(problem)

	switch status {
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		if msg == "" {
			msg = "Staff/HR rejected the submitted employee fields"
		}
		return &InvalidInput{Message: msg}
	case http.StatusNotFound:
		return ErrNotFound
	case http.StatusConflict:
		if msg != "" {
			return &ConflictError{Message: msg}
		}
		return ErrConflict
	default:
		return ErrUpstreamUnavailable
	}
}

// ConflictError wraps ErrConflict with a safe upstream conflict explanation (e.g. duplicate work email).
type ConflictError struct{ Message string }

func (e *ConflictError) Error() string { return e.Message }
func (e *ConflictError) Unwrap() error { return ErrConflict }

func formatProblemMessage(problem upstreamProblemDetails) string {
	if len(problem.Errors) > 0 {
		keys := make([]string, 0, len(problem.Errors))
		for k := range problem.Errors {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, k := range keys {
			if len(problem.Errors[k]) > 0 {
				parts = append(parts, strings.Join(problem.Errors[k], "; "))
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, "; ")
		}
	}
	if strings.TrimSpace(problem.Detail) != "" {
		return strings.TrimSpace(problem.Detail)
	}
	return strings.TrimSpace(problem.Title)
}

func normalizeDepartmentCode(raw, fallback string) (string, error) {
	dept := strings.ToUpper(strings.TrimSpace(raw))
	if dept == "" {
		dept = fallback
	}
	if strings.ContainsRune(dept, 0) || utf8.RuneCountInString(dept) > 20 {
		return "", &InvalidInput{Message: "department code must be between 1 and 20 characters"}
	}
	for _, r := range dept {
		if !((r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_') {
			return "", &InvalidInput{Message: "department code must contain only letters, numbers, '-' or '_'"}
		}
	}
	return dept, nil
}

func validateRequiredText(field, raw string, maxRunes int) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", &InvalidInput{Message: fmt.Sprintf("%s is required", field)}
	}
	if strings.ContainsRune(trimmed, 0) {
		return "", &InvalidInput{Message: fmt.Sprintf("%s contains invalid NUL bytes", field)}
	}
	if utf8.RuneCountInString(trimmed) > maxRunes {
		return "", &InvalidInput{Message: fmt.Sprintf("%s must be at most %d characters", field, maxRunes)}
	}
	return trimmed, nil
}

func normalizeOptionalText(field string, raw *string, maxRunes int) (*string, error) {
	if raw == nil {
		return nil, nil
	}
	trimmed := strings.TrimSpace(*raw)
	if trimmed == "" {
		return nil, nil
	}
	if strings.ContainsRune(trimmed, 0) {
		return nil, &InvalidInput{Message: fmt.Sprintf("%s contains invalid NUL bytes", field)}
	}
	if utf8.RuneCountInString(trimmed) > maxRunes {
		return nil, &InvalidInput{Message: fmt.Sprintf("%s must be at most %d characters", field, maxRunes)}
	}
	return &trimmed, nil
}

func isTimeoutError(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}
