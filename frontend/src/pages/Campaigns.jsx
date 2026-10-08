import { Link } from 'react-router-dom'
import { useAsync, api, useToast } from '../lib/hooks'
import { Card, Button, Badge, Table, EmptyState, Spinner, Stat } from '../components/ui'

const statusColor = {
  draft: 'gray', queued: 'blue', sending: 'indigo', completed: 'green', paused: 'amber', failed: 'red',
}

export default function Campaigns() {
  const { data, loading, refresh } = useAsync('/campaigns')
  const { toast, show } = useToast()

  async function dispatch(id) {
    try {
      await api.post(`/campaigns/${id}/dispatch`, {})
      show('Campaign queued — workers are picking it up')
      refresh()
    } catch (err) {
      show(err.message)
    }
  }

  async function remove(id) {
    if (!confirm('Delete this campaign?')) return
    await api.del(`/campaigns/${id}`)
    show('Campaign deleted')
    refresh()
  }

  const rows = data || []
  const active = rows.filter((c) => ['queued', 'sending'].includes(c.status)).length
  const sentTotal = rows.reduce((a, c) => a + (c.sent_count || 0), 0)

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold text-slate-800">Campaigns</h1>
          <p className="text-sm text-slate-500">Build once, dispatch through throttled SMTP workers, watch progress live.</p>
        </div>
        <Link to="/campaigns/new"><Button>New campaign</Button></Link>
      </div>

      <div className="grid grid-cols-3 gap-4">
        <Stat label="Total" value={rows.length} color="slate" />
        <Stat label="In flight" value={active} color="indigo" />
        <Stat label="Emails sent" value={sentTotal.toLocaleString()} color="green" />
      </div>

      <Card>
        {loading && !data ? <Spinner /> : rows.length === 0 ? (
          <EmptyState title="No campaigns yet" hint="Create your first campaign to start sending."
            action={<Link to="/campaigns/new"><Button>New campaign</Button></Link>} />
        ) : (
          <Table head={['Campaign', 'Status', 'Progress', 'Created', 'Actions']}>
            {rows.map((c) => {
              const pct = c.total_count ? Math.round(((c.sent_count + c.failed_count) / c.total_count) * 100) : 0
              return (
                <tr key={c.id}>
                  <td className="px-3 py-2.5">
                    <Link to={`/campaigns/${c.id}`} className="font-medium text-slate-800 hover:text-slate-950">{c.subject}</Link>
                    <div className="text-xs text-slate-400">{c.preheader || '—'}</div>
                  </td>
                  <td className="px-3 py-2.5"><Badge color={statusColor[c.status] || 'gray'}>{c.status}</Badge></td>
                  <td className="px-3 py-2.5">
                    <div className="flex items-center gap-2">
                      <div className="h-1.5 w-24 overflow-hidden rounded-full bg-slate-100">
                        <div className="h-full rounded-full bg-emerald-500" style={{ width: `${pct}%` }} />
                      </div>
                      <span className="text-xs text-slate-500">{c.sent_count}/{c.total_count}</span>
                    </div>
                  </td>
                  <td className="px-3 py-2.5 text-xs text-slate-400">{(c.created_at || '').slice(0, 10)}</td>
                  <td className="px-3 py-2.5">
                    <div className="flex items-center justify-end gap-2">
                      {(c.status === 'draft' || c.status === 'failed') && (
                        <Button size="sm" onClick={() => dispatch(c.id)}>Send</Button>
                      )}
                      <Link to={`/campaigns/${c.id}`}><Button size="sm" variant="secondary">Open</Button></Link>
                      {(c.status === 'draft' || c.status === 'failed') && (
                        <Button size="sm" variant="danger" onClick={() => remove(c.id)}>Delete</Button>
                      )}
                    </div>
                  </td>
                </tr>
              )
            })}
          </Table>
        )}
      </Card>

      {toast && <div className="fixed bottom-4 right-4 rounded-lg bg-slate-900 px-4 py-2 text-sm text-white shadow-lg">{toast}</div>}
    </div>
  )
}
