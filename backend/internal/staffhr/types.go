// Package staffhr integrates with the City Management Enterprise System (CMES) Staff/HR REST API.
package staffhr

import "errors"

var (
	// ErrNotConfigured indicates STAFF_HR_SERVICE_URL is not configured.
	ErrNotConfigured = errors.New("staff/HR integration service is not configured")
	// ErrUpstreamUnavailable indicates the external Staff/HR API could not be reached or failed.
	ErrUpstreamUnavailable = errors.New("staff/HR service is unavailable")
	// ErrUpstreamTimeout indicates the external Staff/HR API timed out.
	ErrUpstreamTimeout = errors.New("staff/HR service timed out")
	// ErrConflict indicates a uniqueness conflict in Staff/HR (for example duplicate work email).
	ErrConflict = errors.New("staff/HR resource conflict")
	// ErrNotFound indicates the requested department or staff record was not found in Staff/HR.
	ErrNotFound = errors.New("staff/HR resource not found")
)

// InvalidInput describes a validation or request rejection error.
type InvalidInput struct{ Message string }

func (e *InvalidInput) Error() string { return e.Message }

const (
	// MaxEmployeePageSize matches the CMES Staff/HR directory page cap we inspect for department summaries.
	MaxEmployeePageSize = 50
)

// DepartmentHeadcount summarizes active staffing, on-duty count, and vacancies for one city department.
type DepartmentHeadcount struct {
	Code      string `json:"code"`
	Name      string `json:"name"`
	Active    int    `json:"active"`
	OnDuty    int    `json:"on_duty"`
	Vacancies int    `json:"vacancies"`
}

// StaffMember represents one non-payroll employee summary from CMES Staff/HR.
type StaffMember struct {
	StaffNumber        string  `json:"staff_number"`
	DisplayName        string  `json:"display_name"`
	FirstName          string  `json:"first_name"`
	LastName           string  `json:"last_name"`
	Email              string  `json:"email"`
	Phone              *string `json:"phone,omitempty"`
	EmploymentStatus   string  `json:"employment_status"`
	EmploymentType     string  `json:"employment_type"`
	HomeDepartmentCode string  `json:"home_department_code"`
	HomeDepartmentName string  `json:"home_department_name"`
	CurrentRole        *string `json:"current_role,omitempty"`
	CurrentService     *string `json:"current_service,omitempty"`
	CredentialState    *string `json:"credential_state,omitempty"`
}

// StaffOverview combines department employee counts and the filtered staff directory.
type StaffOverview struct {
	DepartmentCode string                `json:"department_code"`
	DepartmentName string                `json:"department_name"`
	ActiveCount    int                   `json:"active_count"`
	OnDutyCount    int                   `json:"on_duty_count"`
	VacanciesCount int                   `json:"vacancies_count"`
	TotalEmployees int                   `json:"total_employees"`
	CitywideActive int                   `json:"citywide_active"`
	Departments    []DepartmentHeadcount `json:"departments"`
	Employees      []StaffMember         `json:"employees"`
}

// AddEmployeeInput is the validated payload submitted by a Transit Administrator to onboard an employee.
type AddEmployeeInput struct {
	FirstName          string  `json:"first_name"`
	LastName           string  `json:"last_name"`
	PreferredName      *string `json:"preferred_name,omitempty"`
	Email              string  `json:"email"`
	DateOfBirth        string  `json:"date_of_birth"`
	Phone              *string `json:"phone,omitempty"`
	CitizenID          *string `json:"citizen_id,omitempty"`
	HireDate           string  `json:"hire_date"`
	EmploymentType     string  `json:"employment_type"`
	EmploymentStatus   string  `json:"employment_status,omitempty"`
	HomeDepartmentCode string  `json:"home_department_code,omitempty"`
}
