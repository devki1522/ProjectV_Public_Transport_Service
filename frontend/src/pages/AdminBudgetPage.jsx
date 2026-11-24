import { useCallback, useEffect, useState } from 'react'
import { AlertCircle, Banknote, CheckCircle2, DollarSign, RefreshCw, Send, Wallet } from 'lucide-react'
import {
  getAdminBudget,
  submitAdminExpense,
  submitAdminFundingRequest,
  submitAdminRevenue,
} from '../api/admin.js'
import { useAuth } from '../auth/authState.js'
import { LoadingRows } from '../components/DataState.jsx'
import { AdminLayout } from '../features/admin/AdminLayout.jsx'
import { useResource } from '../hooks/useResource.js'

function formatCurrency(amount, currency = 'CAD') {
  if (typeof amount !== 'number' || Number.isNaN(amount)) return '—'
  try {
    return new Intl.NumberFormat('en-CA', {
      style: 'currency',
      currency: currency || 'CAD',
    }).format(amount)
  } catch {
    return `$${amount.toFixed(2)} ${currency || 'CAD'}`
  }
}

function validateAmountAndText(amountRaw, textRaw, textLabel) {
  const amount = Number(amountRaw)
  if (!Number.isFinite(amount) || amount <= 0) {
    return { error: 'Enter a valid amount greater than 0.' }
  }
  const text = textRaw.trim()
  if (!text) {
    return { error: `${textLabel} is required.` }
  }
  return { amount, text }
}

export function AdminBudgetPage() {
  const { csrfToken, handleApiError } = useAuth()
  const loadBudget = useCallback((signal) => getAdminBudget(signal), [])
  const budget = useResource('admin-budget-transit', loadBudget)

  const [expenseForm, setExpenseForm] = useState({ amount: '', description: '' })
  const [expensePending, setExpensePending] = useState(false)
  const [expenseError, setExpenseError] = useState(null)

  const [revenueForm, setRevenueForm] = useState({ amount: '', description: '' })
  const [revenuePending, setRevenuePending] = useState(false)
  const [revenueError, setRevenueError] = useState(null)

  const [fundingForm, setFundingForm] = useState({ amount: '', reason: '', urgent: false })
  const [fundingPending, setFundingPending] = useState(false)
  const [fundingError, setFundingError] = useState(null)

  const [actionNotice, setActionNotice] = useState(null)

  useEffect(() => {
    if (budget.error) {
      handleApiError(budget.error)
    }
  }, [budget.error, handleApiError])

  const visibleError = budget.error && budget.error.status !== 401 ? budget.error : null
  const currency = budget.data?.currency || 'CAD'

  async function handleExpenseSubmit(event) {
    event.preventDefault()
    setExpenseError(null)
    setActionNotice(null)

    const validated = validateAmountAndText(expenseForm.amount, expenseForm.description, 'Expense description')
    if (validated.error) {
      setExpenseError(validated.error)
      return
    }

    setExpensePending(true)
    try {
      const created = await submitAdminExpense(
        {
          department_code: 'TRANSIT',
          amount: validated.amount,
          description: validated.text,
        },
        csrfToken,
      )
      setExpenseForm({ amount: '', description: '' })
      setActionNotice(
        `Expense ${formatCurrency(created?.amount ?? validated.amount, currency)} (${created?.status || 'approved'}) recorded for TRANSIT.`,
      )
      budget.retry()
    } catch (error) {
      if (!handleApiError(error)) {
        setExpenseError(error.message)
      }
    } finally {
      setExpensePending(false)
    }
  }

  async function handleRevenueSubmit(event) {
    event.preventDefault()
    setRevenueError(null)
    setActionNotice(null)

    const validated = validateAmountAndText(revenueForm.amount, revenueForm.description, 'Revenue description')
    if (validated.error) {
      setRevenueError(validated.error)
      return
    }

    setRevenuePending(true)
    try {
      const created = await submitAdminRevenue(
        {
          source: 'FARES',
          department_code: 'TRANSIT',
          amount: validated.amount,
          description: validated.text,
        },
        csrfToken,
      )
      setRevenueForm({ amount: '', description: '' })
      setActionNotice(
        `Fare revenue ${formatCurrency(created?.amount ?? validated.amount, currency)} reported under FARES.`,
      )
      budget.retry()
    } catch (error) {
      if (!handleApiError(error)) {
        setRevenueError(error.message)
      }
    } finally {
      setRevenuePending(false)
    }
  }

  async function handleFundingSubmit(event) {
    event.preventDefault()
    setFundingError(null)
    setActionNotice(null)

    const validated = validateAmountAndText(fundingForm.amount, fundingForm.reason, 'Funding request reason')
    if (validated.error) {
      setFundingError(validated.error)
      return
    }

    setFundingPending(true)
    try {
      const created = await submitAdminFundingRequest(
        {
          department_code: 'TRANSIT',
          amount: validated.amount,
          reason: validated.text,
          urgent: Boolean(fundingForm.urgent),
        },
        csrfToken,
      )
      setFundingForm({ amount: '', reason: '', urgent: false })
      setActionNotice(
        `Funding request for ${formatCurrency(created?.amount ?? validated.amount, currency)} submitted (${created?.status || 'pending'}).`,
      )
    } catch (error) {
      if (!handleApiError(error)) {
        setFundingError(error.message)
      }
    } finally {
      setFundingPending(false)
    }
  }

  return (
    <AdminLayout title="Budget & Finance">
      <div className="admin-page-header">
        <div>
          <p className="eyebrow">Budget &amp; Finance Integration</p>
          <h1>Transit budget &amp; fare revenue</h1>
          <p>Review operating budget allocation, submit transit expenses, report bus fare revenue, and request funding.</p>
        </div>
        <div className="admin-header-actions">
          <button
            className="button button-secondary"
            type="button"
            onClick={budget.retry}
            disabled={budget.pending}
          >
            <RefreshCw size={16} aria-hidden="true" />
            <span>{budget.pending ? 'Refreshing…' : 'Refresh'}</span>
          </button>
        </div>
      </div>

      {actionNotice && (
        <div className="success-notice" role="status">
          <CheckCircle2 size={18} aria-hidden="true" />
          <span>{actionNotice}</span>
        </div>
      )}

      <section className="admin-panel simulator-card" aria-label="Transit operating budget summary">
        {visibleError ? (
          <div className="error-notice" role="alert">
            <AlertCircle size={20} aria-hidden="true" />
            <div>
              <strong>Budget allocation couldn’t be loaded</strong>
              <p>{visibleError.message}</p>
            </div>
            <button className="button button-secondary" type="button" onClick={budget.retry}>
              <RefreshCw size={16} aria-hidden="true" />
              <span>Try again</span>
            </button>
          </div>
        ) : budget.pending && !budget.data ? (
          <LoadingRows label="Loading transit operating budget" />
        ) : budget.data ? (
          <div className="enterprise-metrics-grid">
            <article className="metric-card">
              <span className="metric-label">Allocated budget</span>
              <strong className="metric-value">{formatCurrency(budget.data.allocated, currency)}</strong>
              <span className="metric-hint">Department {budget.data.department_code || 'TRANSIT'}</span>
            </article>
            <article className="metric-card">
              <span className="metric-label">Spent to date</span>
              <strong className="metric-value">{formatCurrency(budget.data.spent, currency)}</strong>
              <span className="metric-hint">Approved operating expenses</span>
            </article>
            <article className="metric-card">
              <span className="metric-label">Remaining balance</span>
              <strong className="metric-value metric-value-accent">
                {formatCurrency(budget.data.remaining, currency)}
              </strong>
              <span className="metric-hint">Available in {currency}</span>
            </article>
          </div>
        ) : null}

        <div className="admin-panel-footer">
          <Wallet size={16} aria-hidden="true" />
          <span>Connected to Budget &amp; Finance endpoints (GET /allocations/transit, POST /expenses, POST /revenue, POST /funding-requests).</span>
        </div>
      </section>

      <div className="admin-fleet-grid">
        <section className="admin-panel" aria-labelledby="submit-expense-heading">
          <div className="admin-panel-header">
            <h2 id="submit-expense-heading">Submit operating expense</h2>
            <p>Send equipment, overtime, or fuel expenses for department TRANSIT (PT-FR-022).</p>
          </div>

          {expenseError && (
            <div className="error-notice auth-error" role="alert">
              <AlertCircle size={18} aria-hidden="true" />
              <div>
                <strong>Expense rejected or failed</strong>
                <p>{expenseError}</p>
              </div>
            </div>
          )}

          <form className="admin-form" onSubmit={handleExpenseSubmit} noValidate>
            <div className="field">
              <label htmlFor="budget-expense-amount">Amount (CAD)</label>
              <input
                id="budget-expense-amount"
                type="number"
                step="0.01"
                min="0.01"
                placeholder="1000.00"
                value={expenseForm.amount}
                onChange={(event) => setExpenseForm((c) => ({ ...c, amount: event.target.value }))}
                required
              />
            </div>

            <div className="field">
              <label htmlFor="budget-expense-description">Description</label>
              <input
                id="budget-expense-description"
                type="text"
                maxLength={250}
                placeholder="Fuel, overtime, or fleet maintenance"
                value={expenseForm.description}
                onChange={(event) => setExpenseForm((c) => ({ ...c, description: event.target.value }))}
                required
              />
            </div>

            <div className="admin-form-actions">
              <button className="button button-primary" type="submit" disabled={expensePending}>
                <DollarSign size={16} aria-hidden="true" />
                <span>{expensePending ? 'Submitting expense…' : 'Submit expense'}</span>
              </button>
            </div>
          </form>
        </section>

        <section className="admin-panel" aria-labelledby="report-revenue-heading">
          <div className="admin-panel-header">
            <h2 id="report-revenue-heading">Report bus fare revenue</h2>
            <p>Report collected transit fare revenue under source FARES (PT-FR-024).</p>
          </div>

          {revenueError && (
            <div className="error-notice auth-error" role="alert">
              <AlertCircle size={18} aria-hidden="true" />
              <div>
                <strong>Unable to report fare revenue</strong>
                <p>{revenueError}</p>
              </div>
            </div>
          )}

          <form className="admin-form" onSubmit={handleRevenueSubmit} noValidate>
            <div className="field">
              <label htmlFor="budget-revenue-amount">Amount (CAD)</label>
              <input
                id="budget-revenue-amount"
                type="number"
                step="0.01"
                min="0.01"
                placeholder="4320.00"
                value={revenueForm.amount}
                onChange={(event) => setRevenueForm((c) => ({ ...c, amount: event.target.value }))}
                required
              />
            </div>

            <div className="field">
              <label htmlFor="budget-revenue-description">Description</label>
              <input
                id="budget-revenue-description"
                type="text"
                maxLength={250}
                placeholder="Daily bus farebox collection"
                value={revenueForm.description}
                onChange={(event) => setRevenueForm((c) => ({ ...c, description: event.target.value }))}
                required
              />
            </div>

            <div className="admin-form-actions">
              <button className="button button-primary" type="submit" disabled={revenuePending}>
                <Banknote size={16} aria-hidden="true" />
                <span>{revenuePending ? 'Reporting revenue…' : 'Report fare revenue'}</span>
              </button>
            </div>
          </form>
        </section>
      </div>

      <section className="admin-panel" aria-labelledby="funding-request-heading">
        <div className="admin-panel-header">
          <h2 id="funding-request-heading">Request additional funding</h2>
          <p>Submit a funding request to Budget &amp; Finance if TRANSIT approaches its allocation limit.</p>
        </div>

        {fundingError && (
          <div className="error-notice auth-error" role="alert">
            <AlertCircle size={18} aria-hidden="true" />
            <div>
              <strong>Unable to submit funding request</strong>
              <p>{fundingError}</p>
            </div>
          </div>
        )}

        <form className="admin-form" onSubmit={handleFundingSubmit} noValidate>
          <div className="admin-vehicle-form-row">
            <div className="field">
              <label htmlFor="budget-funding-amount">Requested amount (CAD)</label>
              <input
                id="budget-funding-amount"
                type="number"
                step="0.01"
                min="0.01"
                placeholder="50000.00"
                value={fundingForm.amount}
                onChange={(event) => setFundingForm((c) => ({ ...c, amount: event.target.value }))}
                required
              />
            </div>
            <div className="field">
              <label htmlFor="budget-funding-reason">Reason</label>
              <input
                id="budget-funding-reason"
                type="text"
                maxLength={500}
                placeholder="Winter fleet maintenance overrun"
                value={fundingForm.reason}
                onChange={(event) => setFundingForm((c) => ({ ...c, reason: event.target.value }))}
                required
              />
            </div>
          </div>

          <label className="checkbox-field">
            <input
              id="budget-funding-urgent"
              type="checkbox"
              checked={fundingForm.urgent}
              onChange={(event) => setFundingForm((c) => ({ ...c, urgent: event.target.checked }))}
            />
            <span>Mark request as urgent</span>
          </label>

          <div className="admin-form-actions">
            <button className="button button-secondary" type="submit" disabled={fundingPending}>
              <Send size={16} aria-hidden="true" />
              <span>{fundingPending ? 'Sending request…' : 'Send funding request'}</span>
            </button>
          </div>
        </form>
      </section>
    </AdminLayout>
  )
}
