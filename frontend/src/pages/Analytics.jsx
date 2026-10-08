import { useAsync } from '../lib/hooks'
import { Card, Stat, Spinner } from '../components/ui'

export default function Analytics() {
  const dv = useAsync('/deliverability')
  const ov = useAsync('/overview')

  if (dv.loading && !dv.data) return <Spinner />
  const d = dv.data || {}
  const o = ov.data || {}

  const rate = (v) => `${(v || 0).toFixed(1)}%`
  const ringColor = (v, good) => (v >= good ? 'text-emerald-600' : v > 0 ? 'text-amber-600' : 'text-slate-300')

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-xl font-semibold text-slate-800">Analytics</h1>
        <p className="text-sm text-slate-500">Deliverability health across every campaign — the numbers mailbox providers judge you on.</p>
      </div>

      <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
        <Stat label="Delivered" value={(d.sent || 0).toLocaleString()} color="green" sub={`${(d.failed || 0).toLocaleString()} failed`} />
        <Stat label="Delivery rate" value={rate(d.delivery_rate)} color="slate" />
        <Stat label="Open rate" value={rate(d.open_rate)} color="blue" />
        <Stat label="Click rate" value={rate(d.click_rate)} color="indigo" />
      </div>

      <div className="grid gap-4 lg:grid-cols-2">
        <Card title="Engagement" subtitle="Rates derived from tracked events">
          <div className="space-y-4">
            <Meter label="Delivery rate" value={d.delivery_rate} good={95} suffix="%" />
            <Meter label="Open rate" value={d.open_rate} good={20} suffix="%" />
            <Meter label="Click rate" value={d.click_rate} good={3} suffix="%" />
            <Meter label="Bounce rate" value={d.bounce_rate} good={2} inverse suffix="%" />
          </div>
        </Card>

        <Card title="Event totals" subtitle="Across all campaigns">
          <div className="grid grid-cols-2 gap-3">
            <Mini label="Emails sent" value={(o.emails_sent || 0).toLocaleString()} className={ringColor(o.emails_sent, 1)} />
            <Mini label="Emails failed" value={(o.emails_failed || 0).toLocaleString()} />
            <Mini label="Opens" value={(o.opens || 0).toLocaleString()} />
            <Mini label="Clicks" value={(o.clicks || 0).toLocaleString()} />
            <Mini label="Bounces" value={(o.bounces || 0).toLocaleString()} />
            <Mini label="Subscribers" value={(o.subscribers || 0).toLocaleString()} />
          </div>
          <p className="mt-4 text-xs text-slate-400">
            Open, click and bounce events are recorded via tracking pixels and link redirects wired into each sent message.
          </p>
        </Card>
      </div>
    </div>
  )
}

function Meter({ label, value = 0, good, inverse, suffix = '' }) {
  const pct = Math.max(0, Math.min(100, value))
  const healthy = inverse ? value <= good : value >= good
  const bar = inverse ? (value > good ? 'bg-rose-500' : 'bg-emerald-500') : (healthy ? 'bg-emerald-500' : 'bg-amber-400')
  return (
    <div>
      <div className="mb-1 flex items-center justify-between text-sm">
        <span className="text-slate-600">{label}</span>
        <span className={`font-medium ${healthy ? 'text-slate-800' : 'text-amber-600'}`}>{value.toFixed(1)}{suffix}</span>
      </div>
      <div className="h-2 w-full overflow-hidden rounded-full bg-slate-100">
        <div className={`h-full rounded-full ${bar}`} style={{ width: `${pct}%` }} />
      </div>
    </div>
  )
}

function Mini({ label, value, className = 'text-slate-800' }) {
  return (
    <div className="rounded-lg border border-slate-200 p-3">
      <p className="text-xs text-slate-400">{label}</p>
      <p className={`mt-1 text-lg font-semibold ${className}`}>{value}</p>
    </div>
  )
}
