import { useState } from 'react'
import { useAsync, api, useToast } from '../lib/hooks'
import { Card, Button, Input, Badge, Modal, EmptyState, Spinner } from '../components/ui'

export default function Domains() {
  const { data, loading, refresh } = useAsync('/domains')
  const [open, setOpen] = useState(false)
  const [form, setForm] = useState({ domain: '', selector: 'default', dkim_bits: 2048 })
  const [busy, setBusy] = useState(false)
  const { toast, show } = useToast()

  async function create(e) {
    e.preventDefault()
    setBusy(true)
    try {
      await api.post('/domains', form)
      show('Domain added — publish the DNS records below')
      setOpen(false)
      setForm({ domain: '', selector: 'default', dkim_bits: 2048 })
      await refresh()
    } catch (err) {
      show(err.message)
    } finally {
      setBusy(false)
    }
  }

  async function verify(id) {
    await api.post(`/domains/${id}/verify`, {})
    show('Verification recorded')
    refresh()
  }

  async function remove(id) {
    if (!confirm('Remove this sending domain?')) return
    await api.del(`/domains/${id}`)
    refresh()
  }

  function copy(text) {
    navigator.clipboard?.writeText(text)
    show('Copied to clipboard')
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold text-slate-800">Domains & DNS</h1>
          <p className="text-sm text-slate-500">Every record a receiver checks before trusting your mail. Publish all three to reach the inbox.</p>
        </div>
        <Button onClick={() => setOpen(true)}>Add domain</Button>
      </div>

      {loading && !data ? <Spinner /> : (data || []).length === 0 ? (
        <Card>
          <EmptyState title="No sending domains yet" hint="Add a domain to generate SPF, DKIM and DMARC records."
            action={<Button onClick={() => setOpen(true)}>Add domain</Button>} />
        </Card>
      ) : (
        <div className="space-y-5">
          {(data || []).map((d) => (
            <Card key={d.id} title={d.domain}
              actions={
                <div className="flex items-center gap-2">
                  <Badge color={d.dkim_verified ? 'green' : 'amber'}>DKIM {d.dkim_verified ? 'ok' : 'pending'}</Badge>
                  <Button size="sm" variant="secondary" onClick={() => verify(d.id)}>I've published — verify</Button>
                  <Button size="sm" variant="danger" onClick={() => remove(d.id)}>Remove</Button>
                </div>
              }>
              <p className="mb-3 text-xs text-slate-500">DKIM selector: <code className="rounded bg-slate-100 px-1">{d.selector}</code></p>
              <div className="space-y-3">
                {d.records.map((r, i) => (
                  <div key={i} className="rounded-lg border border-slate-200 bg-slate-50 p-3">
                    <div className="mb-1.5 flex items-center justify-between">
                      <div className="flex items-center gap-2 text-xs font-semibold uppercase tracking-wide text-slate-500">
                        <span className="rounded bg-white px-1.5 py-0.5 text-slate-600">{r.type}</span>
                        {purposeLabel(r.purpose)}
                      </div>
                      <Button size="sm" variant="ghost" onClick={() => copy(r.value)}>Copy value</Button>
                    </div>
                    <div className="grid gap-1 sm:grid-cols-[8rem_1fr]">
                      <Field label="Name" value={r.name} />
                      <Field label="Value" value={r.value} mono />
                    </div>
                    <p className="mt-1.5 text-xs text-slate-400">{r.purpose}</p>
                  </div>
                ))}
              </div>
            </Card>
          ))}
        </div>
      )}

      {toast && <div className="fixed bottom-4 right-4 rounded-lg bg-slate-900 px-4 py-2 text-sm text-white shadow-lg">{toast}</div>}

      <Modal open={open} title="Add sending domain" onClose={() => setOpen(false)}
        footer={<><Button variant="secondary" onClick={() => setOpen(false)}>Cancel</Button><Button form="domain-form" type="submit" disabled={busy}>{busy ? 'Generating keys…' : 'Add domain'}</Button></>}>
        <form id="domain-form" onSubmit={create} className="space-y-3">
          <Input label="Domain" required value={form.domain} onChange={(e) => setForm({ ...form, domain: e.target.value })} placeholder="mail.yourbrand.com" hint="A dedicated subdomain is best practice for reputation." />
          <div className="grid grid-cols-2 gap-3">
            <Input label="DKIM selector" value={form.selector} onChange={(e) => setForm({ ...form, selector: e.target.value })} />
            <Input label="DKIM key bits" type="number" value={form.dkim_bits} onChange={(e) => setForm({ ...form, dkim_bits: Number(e.target.value) })} />
          </div>
          <p className="text-xs text-slate-400">A DKIM keypair is generated for you; the public part is shown as a DNS record after creation.</p>
        </form>
      </Modal>
    </div>
  )
}

function Field({ label, value, mono }) {
  return (
    <div>
      <div className="text-[10px] uppercase tracking-wide text-slate-400">{label}</div>
      <div className={`break-all rounded border border-slate-200 bg-white px-2 py-1 text-xs text-slate-700 ${mono ? 'font-mono' : ''}`}>{value}</div>
    </div>
  )
}

function purposeLabel(purpose = '') {
  if (purpose.startsWith('SPF')) return 'SPF'
  if (purpose.startsWith('DKIM')) return 'DKIM'
  if (purpose.startsWith('DMARC')) return 'DMARC'
  return 'Record'
}
