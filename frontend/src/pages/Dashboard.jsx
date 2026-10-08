import { useEffect } from 'react'
import { Link } from 'react-router-dom'
import { useAsync } from '../lib/hooks'
import { Card, Stat, Spinner, Badge, Button } from '../components/ui'

const statusColor = {
  draft: 'gray', queued: 'blue', sending: 'indigo', completed: 'green', paused: 'amber', failed: 'red',
}

export default function Dashboard() {
  const ov = useAsync('/overview')

  useEffect(() => {
    const id = setInterval(() => ov.refresh(), 8000)
    return () => clearInterval(id)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  if (ov.loading && !ov.data) return <Spinner />
  const d = ov.data || {}

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold text-slate-800">Dashboard</h1>
          <p className="text-sm text-slate-500">Live overview of your sending platform.</p>
        </div>
        <div className="flex gap-2">
          <Link to="/domains"><Button variant="secondary" size="sm">Add domain</Button></Link>
          <Link to="/campaigns/new"><Button size="sm">New campaign</Button></Link>
        </div>
      </div>

      <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
        <Stat label="Subscribers" value={(d.subscribers ?? 0).toLocaleString()} color="slate" />
        <Stat label="Emails sent" value={(d.emails_sent ?? 0).toLocaleString()} color="green" sub={`${(d.emails_failed ?? 0).toLocaleString()} failed`} />
        <Stat label="Campaigns" value={d.campaigns ?? 0} color="indigo" />
        <Stat label="SMTP accounts" value={d.smtp_accounts ?? 0} color="blue" sub={`${d.domains ?? 0} domains`} />
      </div>

      <div className="grid gap-4 lg:grid-cols-2">
        <Card title="Send pipeline" subtitle="Message states across all campaigns">
          <BreakdownBar breakdown={d.messages_by_status || {}} />
        </Card>
        <Card title="Campaigns by status">
          <div className="space-y-2">
            {Object.keys(d.campaigns_by_status || {}).length === 0 && (
              <p className="py-6 text-center text-sm text-slate-400">No campaigns yet.</p>
            )}
            {Object.entries(d.campaigns_by_status || {}).map(([k, v]) => (
              <div key={k} className="flex items-center justify-between text-sm">
                <Badge color={statusColor[k] || 'gray'}>{k}</Badge>
                <span className="font-medium text-slate-700">{v}</span>
              </div>
            ))}
          </div>
        </Card>
      </div>

      <Card title="Getting started" subtitle="Three steps to your first worldwide campaign">
        <ol className="space-y-3 text-sm text-slate-600">
          <Step n="1" title="Connect a sending domain" body="Publish SPF, DKIM and DMARC records to boost inbox placement." to="/domains" label="Domains & DNS" done={(d.domains ?? 0) > 0} />
          <Step n="2" title="Add an SMTP account" body="Bring your own provider (SES, Gmail, Mailgun, Postmark…). Credentials are encrypted at rest." to="/smtp" label="SMTP Accounts" done={(d.smtp_accounts ?? 0) > 0} />
          <Step n="3" title="Import a list & launch" body="Upload subscribers, build a campaign and dispatch it through throttled workers." to="/lists" label="Lists" done={(d.lists ?? 0) > 0} />
        </ol>
      </Card>
    </div>
  )
}

function Step({ n, title, body, to, label, done }) {
  return (
    <li className="flex items-start gap-3">
      <span className={`mt-0.5 grid h-6 w-6 shrink-0 place-items-center rounded-full text-xs font-semibold ${done ? 'bg-emerald-100 text-emerald-700' : 'bg-slate-100 text-slate-500'}`}>
        {done ? '✓' : n}
      </span>
      <div className="flex-1">
        <p className="font-medium text-slate-800">{title}</p>
        <p className="text-slate-500">{body}</p>
      </div>
      <Link to={to}><Button variant="secondary" size="sm">{label}</Button></Link>
    </li>
  )
}

function BreakdownBar({ breakdown }) {
  const colors = { pending: 'bg-slate-300', sent: 'bg-emerald-500', failed: 'bg-rose-500', skipped: 'bg-amber-400', bounced: 'bg-red-500' }
  const total = Object.values(breakdown).reduce((a, b) => a + b, 0)
  if (total === 0) return <p className="py-6 text-center text-sm text-slate-400">No messages sent yet.</p>
  return (
    <div>
      <div className="flex h-3 w-full overflow-hidden rounded-full bg-slate-100">
        {Object.entries(breakdown).map(([k, v]) => (
          <div key={k} className={colors[k] || 'bg-slate-400'} style={{ width: `${(v / total) * 100}%` }} title={`${k}: ${v}`} />
        ))}
      </div>
      <div className="mt-3 flex flex-wrap gap-x-4 gap-y-1 text-xs text-slate-500">
        {Object.entries(breakdown).map(([k, v]) => (
          <span key={k} className="flex items-center gap-1.5">
            <span className={`h-2 w-2 rounded-full ${colors[k] || 'bg-slate-400'}`} />{k} <b className="text-slate-700">{v.toLocaleString()}</b>
          </span>
        ))}
      </div>
    </div>
  )
}
