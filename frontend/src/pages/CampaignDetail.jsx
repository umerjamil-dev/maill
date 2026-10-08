import { useEffect } from 'react'
import { Link, useParams } from 'react-router-dom'
import { useAsync, api, useToast } from '../lib/hooks'
import { Card, Button, Badge, Stat, Spinner } from '../components/ui'

const statusColor = {
  draft: 'gray', queued: 'blue', sending: 'indigo', completed: 'green', paused: 'amber', failed: 'red',
}

const msgColors = {
  pending: 'bg-slate-300', sent: 'bg-emerald-500', failed: 'bg-rose-500',
  skipped: 'bg-amber-400', bounced: 'bg-red-500',
}

export default function CampaignDetail() {
  const { id } = useParams()
  const { data, loading, setData } = useAsync(`/campaigns/${id}`)
  const { toast, show } = useToast()

  // Poll progress while the campaign is in flight.
  useEffect(() => {
    const st = data?.campaign?.status
    if (!['queued', 'sending'].includes(st)) return
    const timer = setInterval(async () => {
      try {
        const res = await api.get(`/campaigns/${id}`)
        setData(res)
      } catch { /* ignore transient poll errors */ }
    }, 3000)
    return () => clearInterval(timer)
  }, [data?.campaign?.status, id, setData])

  if (loading && !data) return <Spinner />
  if (!data) return <Card><p className="text-sm text-slate-500">Campaign not found.</p></Card>

  const c = data.campaign
  const breakdown = data.breakdown || {}
  const total = c.total_count || 0
  const done = c.sent_count + c.failed_count
  const pct = total ? Math.round((done / total) * 100) : 0
  const inFlight = ['queued', 'sending'].includes(c.status)

  async function dispatch() {
    try {
      await api.post(`/campaigns/${id}/dispatch`, {})
      show('Campaign queued')
      const res = await api.get(`/campaigns/${id}`)
      setData(res)
    } catch (err) {
      show(err.message)
    }
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <div className="flex items-center gap-2">
            <h1 className="text-xl font-semibold text-slate-800">{c.subject}</h1>
            <Badge color={statusColor[c.status] || 'gray'}>{c.status}</Badge>
          </div>
          <p className="text-sm text-slate-500">{c.preheader || 'No preheader'}</p>
        </div>
        <div className="flex gap-2">
          {(c.status === 'draft' || c.status === 'failed') && <Button onClick={dispatch}>Launch campaign</Button>}
          <Link to="/campaigns"><Button variant="secondary">Back</Button></Link>
        </div>
      </div>

      {inFlight && (
        <div className="flex items-center gap-2 rounded-lg border border-sky-200 bg-sky-50 px-4 py-2 text-sm text-sky-700">
          <div className="h-4 w-4 animate-spin rounded-full border-2 border-sky-300 border-t-sky-600" />
          Live — progress refreshes every 3 seconds while workers send.
        </div>
      )}

      <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
        <Stat label="Recipients" value={total.toLocaleString()} color="slate" />
        <Stat label="Sent" value={c.sent_count.toLocaleString()} color="green" />
        <Stat label="Failed" value={c.failed_count.toLocaleString()} color="red" />
        <Stat label="Progress" value={`${pct}%`} color="indigo" />
      </div>

      <Card title="Delivery progress" subtitle="Message states across this campaign">
        {total === 0 ? (
          <p className="py-6 text-center text-sm text-slate-400">
            No messages queued yet. {c.status === 'draft' ? 'Launch the campaign to begin.' : ''}
          </p>
        ) : (
          <>
            <div className="flex h-3 w-full overflow-hidden rounded-full bg-slate-100">
              {Object.entries(breakdown).map(([k, v]) => (
                <div key={k} className={msgColors[k] || 'bg-slate-400'} style={{ width: `${(v / total) * 100}%` }} title={`${k}: ${v}`} />
              ))}
            </div>
            <div className="mt-3 flex flex-wrap gap-x-4 gap-y-1 text-xs text-slate-500">
              {Object.entries(breakdown).map(([k, v]) => (
                <span key={k} className="flex items-center gap-1.5">
                  <span className={`h-2 w-2 rounded-full ${msgColors[k] || 'bg-slate-400'}`} />{k} <b className="text-slate-700">{v.toLocaleString()}</b>
                </span>
              ))}
            </div>
          </>
        )}
      </Card>

      {toast && <div className="fixed bottom-4 right-4 rounded-lg bg-slate-900 px-4 py-2 text-sm text-white shadow-lg">{toast}</div>}
    </div>
  )
}
