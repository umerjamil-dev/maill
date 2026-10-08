import { useMemo, useRef, useState } from 'react'
import { useAsync, api, useToast } from '../lib/hooks'
import { Card, Button, Input, Badge, Modal, Table, EmptyState, Spinner } from '../components/ui'

export default function Lists() {
  const { data: lists, loading, refresh } = useAsync('/lists')
  const [selected, setSelected] = useState(null)
  const [open, setOpen] = useState(false)
  const [form, setForm] = useState({ name: '', description: '' })
  const [busy, setBusy] = useState(false)
  const { toast, show } = useToast()

  const active = useMemo(
    () => (lists || []).find((l) => l.id === (selected || lists?.[0]?.id)) || null,
    [lists, selected],
  )

  async function create(e) {
    e.preventDefault()
    setBusy(true)
    try {
      const res = await api.post('/lists', form)
      show('List created')
      setOpen(false)
      setForm({ name: '', description: '' })
      await refresh()
      setSelected(res.id)
    } catch (err) {
      show(err.message)
    } finally {
      setBusy(false)
    }
  }

  async function remove(id) {
    if (!confirm('Delete this list and all its subscribers?')) return
    await api.del(`/lists/${id}`)
    if (selected === id) setSelected(null)
    show('List deleted')
    refresh()
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold text-slate-800">Lists & Subscribers</h1>
          <p className="text-sm text-slate-500">Segment your audience and bulk-import contacts with custom fields.</p>
        </div>
        <Button onClick={() => setOpen(true)}>New list</Button>
      </div>

      <div className="grid gap-4 lg:grid-cols-[20rem_1fr]">
        <Card title="Your lists" className="self-start">
          {loading && !lists ? <Spinner /> : (lists || []).length === 0 ? (
            <EmptyState title="No lists yet" hint="Create a list to start importing subscribers." />
          ) : (
            <ul className="space-y-1">
              {(lists || []).map((l) => {
                const isActive = active?.id === l.id
                return (
                  <li key={l.id}>
                    <button
                      onClick={() => setSelected(l.id)}
                      className={`flex w-full items-center justify-between rounded-lg px-3 py-2 text-left text-sm transition ${
                        isActive ? 'bg-slate-900 text-white' : 'text-slate-700 hover:bg-slate-100'
                      }`}
                    >
                      <span className="truncate">{l.name}</span>
                      <span className={`ml-2 shrink-0 rounded-full px-2 py-0.5 text-xs ${isActive ? 'bg-white/20' : 'bg-slate-100 text-slate-500'}`}>
                        {l.subscriber_count}
                      </span>
                    </button>
                  </li>
                )
              })}
            </ul>
          )}
        </Card>

        {active ? (
          <SubscriberPanel list={active} onCountChange={refresh} show={show} />
        ) : (
          <Card><EmptyState title="Select a list" hint="Choose a list on the left to view and import its subscribers." /></Card>
        )}
      </div>

      {toast && <div className="fixed bottom-4 right-4 rounded-lg bg-slate-900 px-4 py-2 text-sm text-white shadow-lg">{toast}</div>}

      <Modal open={open} title="New list" onClose={() => setOpen(false)}
        footer={<><Button variant="secondary" onClick={() => setOpen(false)}>Cancel</Button><Button form="list-form" type="submit" disabled={busy}>{busy ? 'Creating…' : 'Create list'}</Button></>}>
        <form id="list-form" onSubmit={create} className="space-y-3">
          <Input label="Name" required value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} placeholder="Newsletter subscribers" />
          <Input label="Description" value={form.description} onChange={(e) => setForm({ ...form, description: e.target.value })} />
        </form>
      </Modal>
    </div>
  )
}

function SubscriberPanel({ list, onCountChange, show }) {
  const { data, loading, refresh } = useAsync(`/lists/${list.id}/subscribers`)
  const [addOpen, setAddOpen] = useState(false)
  const [row, setRow] = useState({ email: '', name: '' })
  const [busy, setBusy] = useState(false)
  const [importing, setImporting] = useState(false)
  const fileRef = useRef(null)

  async function addOne(e) {
    e.preventDefault()
    setBusy(true)
    try {
      await api.post(`/lists/${list.id}/subscribers`, { email: row.email, name: row.name, fields: {} })
      setRow({ email: '', name: '' })
      setAddOpen(false)
      show('Subscriber added')
      refresh(); onCountChange()
    } catch (err) {
      show(err.message)
    } finally {
      setBusy(false)
    }
  }

  async function removeSub(id) {
    if (!confirm('Remove this subscriber?')) return
    await api.del(`/subscribers/${id}`)
    refresh(); onCountChange()
  }

  async function onFile(e) {
    const file = e.target.files?.[0]
    if (!file) return
    setImporting(true)
    try {
      const text = await file.text()
      const subs = parseCSV(text)
      if (subs.length === 0) {
        show('No rows found — CSV needs an "email" column')
        return
      }
      const res = await api.post(`/lists/${list.id}/subscribers/bulk`, { subscribers: subs })
      show(`Imported ${res.added} subscriber(s)${res.skipped ? `, skipped ${res.skipped}` : ''}`)
      refresh(); onCountChange()
    } catch (err) {
      show(err.message)
    } finally {
      setImporting(false)
      if (fileRef.current) fileRef.current.value = ''
    }
  }

  return (
    <Card
      title={list.name}
      subtitle={`${list.subscriber_count} subscriber(s)`}
      actions={
        <div className="flex gap-2">
          <label className="inline-flex cursor-pointer items-center gap-1.5 rounded-lg border border-slate-300 bg-white px-2.5 py-1.5 text-xs font-medium text-slate-700 hover:bg-slate-100">
            {importing ? 'Importing…' : 'Import CSV'}
            <input ref={fileRef} type="file" accept=".csv,text/csv" className="hidden" onChange={onFile} disabled={importing} />
          </label>
          <Button size="sm" onClick={() => setAddOpen(true)}>Add</Button>
        </div>
      }
    >
      {loading && !data ? <Spinner /> : (data || []).length === 0 ? (
        <EmptyState title="No subscribers yet" hint='Import a CSV with an "email" column, or add one manually.' />
      ) : (
        <Table head={['Email', 'Name', 'Fields', 'Status', '']}>
          {(data || []).map((s) => (
            <tr key={s.id}>
              <td className="px-3 py-2.5 font-medium text-slate-800">{s.email}</td>
              <td className="px-3 py-2.5 text-slate-600">{s.name || '—'}</td>
              <td className="px-3 py-2.5 text-xs text-slate-400">
                {Object.keys(s.fields || {}).length
                  ? Object.entries(s.fields).slice(0, 3).map(([k, v]) => `${k}: ${v}`).join(' · ')
                  : '—'}
              </td>
              <td className="px-3 py-2.5">
                <Badge color={s.unsubscribed ? 'red' : 'green'}>{s.unsubscribed ? 'unsubscribed' : 'active'}</Badge>
              </td>
              <td className="px-3 py-2.5 text-right">
                <Button size="sm" variant="danger" onClick={() => removeSub(s.id)}>Remove</Button>
              </td>
            </tr>
          ))}
        </Table>
      )}

      <Modal open={addOpen} title="Add subscriber" onClose={() => setAddOpen(false)}
        footer={<><Button variant="secondary" onClick={() => setAddOpen(false)}>Cancel</Button><Button form="sub-form" type="submit" disabled={busy}>{busy ? 'Saving…' : 'Add'}</Button></>}>
        <form id="sub-form" onSubmit={addOne} className="space-y-3">
          <Input label="Email" type="email" required value={row.email} onChange={(e) => setRow({ ...row, email: e.target.value })} />
          <Input label="Name" value={row.name} onChange={(e) => setRow({ ...row, name: e.target.value })} />
        </form>
      </Modal>
    </Card>
  )
}

// parseCSV turns CSV text into subscriber objects. Requires an "email" column;
// a "name" column and any other columns become merge-tag fields.
export function parseCSV(text) {
  const lines = text.split(/\r?\n/).filter((l) => l.trim())
  if (lines.length === 0) return []
  const rows = lines.map(splitCSVLine)
  const header = rows[0].map((h) => h.trim().toLowerCase())
  const emailIdx = header.indexOf('email')
  if (emailIdx === -1) return []
  const nameIdx = header.indexOf('name')

  const out = []
  for (const r of rows.slice(1)) {
    const email = (r[emailIdx] || '').trim()
    if (!email || !email.includes('@')) continue
    const fields = {}
    header.forEach((col, i) => {
      if (col === 'email' || col === 'name' || !col) return
      if (r[i] && r[i].trim()) fields[col] = r[i].trim()
    })
    out.push({ email, name: nameIdx >= 0 ? (r[nameIdx] || '').trim() : '', fields })
  }
  return out
}

function splitCSVLine(line) {
  const out = []
  let cur = ''
  let quoted = false
  for (let i = 0; i < line.length; i++) {
    const ch = line[i]
    if (quoted) {
      if (ch === '"' && line[i + 1] === '"') { cur += '"'; i++ }
      else if (ch === '"') quoted = false
      else cur += ch
    } else if (ch === '"') quoted = true
    else if (ch === ',') { out.push(cur); cur = '' }
    else cur += ch
  }
  out.push(cur)
  return out
}
