import { useCallback, useEffect, useMemo, useState } from 'react'
import { AlertCircle, CheckCircle2, RefreshCw, UserPlus, Users } from 'lucide-react'
import { addAdminEmployee, getAdminStaff } from '../api/admin.js'
import { useAuth } from '../auth/authState.js'
import { EmptyState, LoadingRows } from '../components/DataState.jsx'
import { AdminLayout } from '../features/admin/AdminLayout.jsx'
import { useResource } from '../hooks/useResource.js'

const EMPLOYMENT_TYPES = [
  { value: 'FullTime', label: 'Full-time' },
  { value: 'PartTime', label: 'Part-time' },
  { value: 'Contract', label: 'Contract' },
  { value: 'Seasonal', label: 'Seasonal' },
]

const ONBOARDING_STATUSES = [
  { value: 'Active', label: 'Active' },
  { value: 'Probation', label: 'Probation' },
]

function todayDateInput() {
  const now = new Date()
  const year = now.getFullYear()
  const month = String(now.getMonth() + 1).padStart(2, '0')
  const day = String(now.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}

function buildInitialEmployeeForm(departmentCode = 'PTR') {
  return {
    firstName: '',
    lastName: '',
    preferredName: '',
    email: '',
    dateOfBirth: '',
    hireDate: todayDateInput(),
    phone: '',
    employmentType: 'FullTime',
    employmentStatus: 'Active',
    homeDepartmentCode: departmentCode,
  }
}

function validateEmployeeForm(form) {
  const firstName = form.firstName.trim()
  const lastName = form.lastName.trim()
  const email = form.email.trim()
  const dateOfBirth = form.dateOfBirth.trim()
  const hireDate = form.hireDate.trim()

  if (!firstName || [...firstName].length > 100) {
    return { error: 'First name is required and must be 100 characters or fewer.' }
  }
  if (!lastName || [...lastName].length > 100) {
    return { error: 'Last name is required and must be 100 characters or fewer.' }
  }
  if (!email || !email.includes('@') || email.startsWith('@') || email.endsWith('@')) {
    return { error: 'A valid work email address is required.' }
  }
  if (!/^\d{4}-\d{2}-\d{2}$/.test(dateOfBirth)) {
    return { error: 'Date of birth is required (YYYY-MM-DD).' }
  }
  if (!/^\d{4}-\d{2}-\d{2}$/.test(hireDate)) {
    return { error: 'Hire date is required (YYYY-MM-DD).' }
  }

  const dobDate = new Date(`${dateOfBirth}T00:00:00Z`)
  const today = new Date(`${todayDateInput()}T00:00:00Z`)
  if (Number.isNaN(dobDate.getTime()) || dobDate >= today) {
    return { error: 'Date of birth must be in the past.' }
  }

  const hire = new Date(`${hireDate}T00:00:00Z`)
  if (Number.isNaN(hire.getTime())) {
    return { error: 'Hire date must be a valid calendar date.' }
  }
  const minHireDate = new Date(Date.UTC(dobDate.getUTCFullYear() + 16, dobDate.getUTCMonth(), dobDate.getUTCDate()))
  if (hire < minHireDate) {
    return { error: 'Hire date must be at least 16 years after date of birth.' }
  }

  const payload = {
    first_name: firstName,
    last_name: lastName,
    email,
    date_of_birth: dateOfBirth,
    hire_date: hireDate,
    employment_type: form.employmentType,
    employment_status: form.employmentStatus,
    home_department_code: form.homeDepartmentCode || 'PTR',
  }
  if (form.preferredName.trim()) payload.preferred_name = form.preferredName.trim()
  if (form.phone.trim()) payload.phone = form.phone.trim()

  return { payload }
}

function statusPillClass(status) {
  switch (status) {
    case 'Active':
      return 'is-active'
    case 'Suspended':
    case 'Terminated':
      return 'is-delayed'
    default:
      return 'is-inactive'
  }
}

export function AdminStaffPage() {
  const { csrfToken, handleApiError } = useAuth()
  const [department, setDepartment] = useState('PTR')
  const [form, setForm] = useState(() => buildInitialEmployeeForm('PTR'))
  const [submitPending, setSubmitPending] = useState(false)
  const [submitError, setSubmitError] = useState(null)
  const [actionNotice, setActionNotice] = useState(null)

  const loadStaff = useCallback((signal) => getAdminStaff(department, signal), [department])
  const staff = useResource(`admin-staff-${department}`, loadStaff)

  useEffect(() => {
    if (staff.error?.status === 401 || staff.error?.status === 403) {
      handleApiError(staff.error)
    }
  }, [staff.error, handleApiError])

  const departmentOptions = useMemo(() => {
    const list = staff.data?.departments ?? []
    if (list.length > 0) return list
    return [{ code: 'PTR', name: 'Public Transport' }]
  }, [staff.data])

  const employees = staff.data?.employees ?? []
  const visibleError =
    staff.error && staff.error.status !== 401 && staff.error.status !== 403 ? staff.error : null

  function handleDepartmentChange(nextCode) {
    setDepartment(nextCode)
    setForm((current) => ({ ...current, homeDepartmentCode: nextCode }))
  }

  async function handleSubmit(event) {
    event.preventDefault()
    if (submitPending) return

    const { payload, error: validationError } = validateEmployeeForm(form)
    if (validationError) {
      setSubmitError(validationError)
      setActionNotice(null)
      return
    }

    setSubmitPending(true)
    setSubmitError(null)
    setActionNotice(null)
    try {
      const created = await addAdminEmployee(payload, csrfToken)
      setActionNotice(
        `Added employee ${created.display_name || `${created.first_name} ${created.last_name}`} (${created.staff_number}) to ${created.home_department_code}.`,
      )
      setForm(buildInitialEmployeeForm(department))
      staff.retry()
    } catch (error) {
      if (error.status === 401 || error.status === 403) {
        handleApiError(error)
        return
      }
      setSubmitError(error.message)
    } finally {
      setSubmitPending(false)
    }
  }

  return (
    <AdminLayout title="Staff & HR">
      <div className="page-heading admin-page-heading">
        <div>
          <h1>Staff &amp; HR directory</h1>
          <p>Live workforce headcount and employee onboarding via CMES Staff/HR REST integration.</p>
        </div>
        <div className="admin-filter-controls">
          <div className="field">
            <label htmlFor="staff-department-filter">Department</label>
            <select
              id="staff-department-filter"
              value={department}
              onChange={(event) => handleDepartmentChange(event.target.value)}
            >
              {departmentOptions.map((dept) => (
                <option key={dept.code} value={dept.code}>
                  {dept.code} · {dept.name}
                </option>
              ))}
            </select>
          </div>
          <button
            className="button button-secondary"
            type="button"
            onClick={staff.retry}
            disabled={staff.pending}
          >
            <RefreshCw size={16} aria-hidden="true" />
            <span>{staff.pending ? 'Refreshing…' : 'Refresh'}</span>
          </button>
        </div>
      </div>

      {actionNotice && (
        <div className="success-notice" role="status">
          <CheckCircle2 size={18} aria-hidden="true" />
          <span>{actionNotice}</span>
        </div>
      )}

      <section className="admin-panel simulator-card" aria-label="Workforce headcount summary">
        {visibleError ? (
          <div className="error-notice" role="alert">
            <AlertCircle size={20} aria-hidden="true" />
            <div>
              <strong>Staff &amp; HR summary couldn’t be loaded</strong>
              <p>{visibleError.message}</p>
            </div>
            <button className="button button-secondary" type="button" onClick={staff.retry}>
              <RefreshCw size={16} aria-hidden="true" />
              <span>Try again</span>
            </button>
          </div>
        ) : staff.pending && !staff.data ? (
          <LoadingRows label="Loading employee counts" />
        ) : staff.data ? (
          <div className="enterprise-metrics-grid">
            <article className="metric-card">
              <span className="metric-label">Active employees</span>
              <strong className="metric-value metric-value-accent">{staff.data.active_count}</strong>
              <span className="metric-hint">
                {staff.data.department_code} · {staff.data.department_name}
              </span>
            </article>
            <article className="metric-card">
              <span className="metric-label">On duty now</span>
              <strong className="metric-value">{staff.data.on_duty_count}</strong>
              <span className="metric-hint">Staff currently on active shift</span>
            </article>
            <article className="metric-card">
              <span className="metric-label">Open vacancies</span>
              <strong className="metric-value">{staff.data.vacancies_count}</strong>
              <span className="metric-hint">Approved unfilled positions</span>
            </article>
            <article className="metric-card">
              <span className="metric-label">Directory total</span>
              <strong className="metric-value">{staff.data.total_employees}</strong>
              <span className="metric-hint">{staff.data.citywide_active} active across all city departments</span>
            </article>
          </div>
        ) : null}

        <div className="admin-panel-footer">
          <Users size={16} aria-hidden="true" />
          <span>Headcount and employee profiles come directly from the Staff/HR REST service without exposing salary data.</span>
        </div>
      </section>

      <div className="admin-fleet-grid">
        <section className="admin-panel" aria-labelledby="add-employee-heading">
          <div className="admin-panel-header">
            <h2 id="add-employee-heading">Add employee</h2>
            <p>Onboard a new employee and generate a city Staff Number.</p>
          </div>

          {submitError && (
            <div className="error-notice auth-error" role="alert">
              <AlertCircle size={18} aria-hidden="true" />
              <div>
                <strong>Unable to add employee</strong>
                <p>{submitError}</p>
              </div>
            </div>
          )}

          <form className="admin-form" onSubmit={handleSubmit} noValidate>
            <div className="admin-vehicle-form-row">
              <div className="field">
                <label htmlFor="emp-first-name">First name</label>
                <input
                  id="emp-first-name"
                  type="text"
                  maxLength={100}
                  value={form.firstName}
                  onChange={(event) => setForm((c) => ({ ...c, firstName: event.target.value }))}
                  required
                />
              </div>
              <div className="field">
                <label htmlFor="emp-last-name">Last name</label>
                <input
                  id="emp-last-name"
                  type="text"
                  maxLength={100}
                  value={form.lastName}
                  onChange={(event) => setForm((c) => ({ ...c, lastName: event.target.value }))}
                  required
                />
              </div>
            </div>

            <div className="admin-vehicle-form-row">
              <div className="field">
                <label htmlFor="emp-preferred-name">Preferred name (optional)</label>
                <input
                  id="emp-preferred-name"
                  type="text"
                  maxLength={100}
                  value={form.preferredName}
                  onChange={(event) => setForm((c) => ({ ...c, preferredName: event.target.value }))}
                />
              </div>
              <div className="field">
                <label htmlFor="emp-phone">Phone (optional)</label>
                <input
                  id="emp-phone"
                  type="text"
                  maxLength={30}
                  placeholder="555-0199"
                  value={form.phone}
                  onChange={(event) => setForm((c) => ({ ...c, phone: event.target.value }))}
                />
              </div>
            </div>

            <div className="field">
              <label htmlFor="emp-email">Work email</label>
              <input
                id="emp-email"
                type="email"
                placeholder="jordan.chen@cmes.example"
                value={form.email}
                onChange={(event) => setForm((c) => ({ ...c, email: event.target.value }))}
                required
              />
            </div>

            <div className="admin-vehicle-form-row">
              <div className="field">
                <label htmlFor="emp-dob">Date of birth</label>
                <input
                  id="emp-dob"
                  type="date"
                  value={form.dateOfBirth}
                  onChange={(event) => setForm((c) => ({ ...c, dateOfBirth: event.target.value }))}
                  required
                />
              </div>
              <div className="field">
                <label htmlFor="emp-hire-date">Hire date</label>
                <input
                  id="emp-hire-date"
                  type="date"
                  value={form.hireDate}
                  onChange={(event) => setForm((c) => ({ ...c, hireDate: event.target.value }))}
                  required
                />
              </div>
            </div>

            <div className="admin-vehicle-form-row">
              <div className="field">
                <label htmlFor="emp-type">Employment type</label>
                <select
                  id="emp-type"
                  value={form.employmentType}
                  onChange={(event) => setForm((c) => ({ ...c, employmentType: event.target.value }))}
                >
                  {EMPLOYMENT_TYPES.map((option) => (
                    <option key={option.value} value={option.value}>
                      {option.label}
                    </option>
                  ))}
                </select>
              </div>
              <div className="field">
                <label htmlFor="emp-status">Initial status</label>
                <select
                  id="emp-status"
                  value={form.employmentStatus}
                  onChange={(event) => setForm((c) => ({ ...c, employmentStatus: event.target.value }))}
                >
                  {ONBOARDING_STATUSES.map((option) => (
                    <option key={option.value} value={option.value}>
                      {option.label}
                    </option>
                  ))}
                </select>
              </div>
            </div>

            <div className="field">
              <label htmlFor="emp-department">Home department</label>
              <select
                id="emp-department"
                value={form.homeDepartmentCode}
                onChange={(event) => setForm((c) => ({ ...c, homeDepartmentCode: event.target.value }))}
              >
                {departmentOptions.map((dept) => (
                  <option key={dept.code} value={dept.code}>
                    {dept.code} · {dept.name}
                  </option>
                ))}
              </select>
            </div>

            <button className="button button-primary" type="submit" disabled={submitPending}>
              <UserPlus size={16} aria-hidden="true" />
              <span>{submitPending ? 'Adding employee...' : 'Add employee'}</span>
            </button>
          </form>
        </section>

        <section className="admin-panel" aria-labelledby="staff-directory-heading">
          <div className="admin-catalog-toolbar">
            <div>
              <h2 id="staff-directory-heading">Department directory</h2>
              <p>
                {staff.data
                  ? `${employees.length} shown of ${staff.data.total_employees} in ${staff.data.department_name}`
                  : 'Loading staff directory…'}
              </p>
            </div>
          </div>

          {staff.pending && !staff.data ? (
            <LoadingRows label="Loading staff directory" />
          ) : employees.length === 0 ? (
            <EmptyState
              title="No employees in this department"
              description="Use the Add employee form to onboard staff members into this department."
            />
          ) : (
            <div className="admin-table-wrap">
              <table className="admin-table">
                <thead>
                  <tr>
                    <th scope="col">Staff #</th>
                    <th scope="col">Employee</th>
                    <th scope="col">Role &amp; service</th>
                    <th scope="col">Type</th>
                    <th scope="col">Status</th>
                    <th scope="col">Credentials</th>
                  </tr>
                </thead>
                <tbody>
                  {employees.map((member) => (
                    <tr key={member.staff_number}>
                      <td className="tabular-cell">
                        <strong>{member.staff_number}</strong>
                      </td>
                      <td>
                        <div className="admin-vehicle-cell">
                          <strong>{member.display_name || `${member.first_name} ${member.last_name}`}</strong>
                          <small>{member.email}</small>
                        </div>
                      </td>
                      <td>
                        <div className="admin-vehicle-cell">
                          <strong>{member.current_role || 'Unassigned'}</strong>
                          <small>{member.current_service || member.home_department_name}</small>
                        </div>
                      </td>
                      <td>{member.employment_type}</td>
                      <td>
                        <span className={`status-pill ${statusPillClass(member.employment_status)}`}>
                          {member.employment_status}
                        </span>
                      </td>
                      <td>{member.credential_state || 'None'}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </section>
      </div>
    </AdminLayout>
  )
}
