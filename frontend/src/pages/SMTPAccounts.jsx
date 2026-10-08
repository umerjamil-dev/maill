import { useState } from 'react'
import { useAsync, api } from '../lib/hooks'
import { Card, Button, Input, Select, Badge, Modal, Table, EmptyState, Spinner } from '../components/ui'
import { useToast } from '../lib/hooks'

const empty = {
  host: '', port: 587, username: '', password: '', from_email: '', from_name: '',
  auth_method: 'login', tls_mode: 'starttls', max_emails_per_min: 60, rotation_weight: 1,
}

export default function SMTPAccounts() {
  const { data, loading, refresh } = useAsync('/smtp-accounts')
  const [open, setOpen] = useState(false)
  const [form, setForm] = useState(empty)
  const [busy, setBusy] = useState(false)
  const [testTarget, setTestTarget] = useState({})
  const { toast, show } = useToast()

  const totalRate = (data || []).reduce((a, s) => a + (s.max_emails_per_min || 0), 0)

  async function create(e) {
    e.preventDefault()
    setBusy(true)
    try {
      await api.post('/smtp-accounts', form)
      show('SMTP account added')
      setOpen(false)
      setForm(empty)
      await refresh()
    } catch (err) {
      show(err.message)
    } finally {
      setBusy(false)
    }
  }

  async function remove(id) {
    if (!confirm('Remove this SMTP account?')) return
    await api.del(`/smtp-accounts/${id}`)
    show('Account removed')
    refresh()
  }

  async function test(id) {
    const to = testTarget[id]
    if (!to) return show('Enter a test recipient first')
    setBusy(true)
    try {
      const res = await api.post(`/smtp-accounts/${id}/test`, { to })
      show(res.verified ? `Test delivered to ${to}` : 'Failed')
      refresh()
    } catch (err) {
      show(`Test failed: ${err.message}`)
    } finally {
      setBusy(false)
    }
  }

  function set(k) {
    return (e) => setForm({ ...form, [k]: e.target.value })
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold text-slate-800">SMTP Accounts</h1>
          <p className="text-sm text-slate-500">
            Connect providers and send in parallel — accounts rotate by weight, each with its own throttle.
          </p>
        </div>
        <Button onClick={() => setOpen(true)}>Add account</Button>
      </div>

      <div className="grid grid-cols-2 gap-4 lg:grid-cols-3">
        <Card className="!p-0"><div className="p-5"><p className="text-xs uppercase text-slate-400">Connected</p><p className="mt-1 text-2xl font-semibold">{data?.length ?? 0}</p></div></Card>
        <Card className="!p-0"><div className="p-5"><p className="text-xs uppercase text-slate-400">Verified</p><p className="mt-1 text-2xl font-semibold text-emerald-600">{(data || []).filter((a) => a.verified).length}</p></div></Card>
        <Card className="!p-0"><div className="p-5"><p className="text-xs uppercase text-slate-400">Combined rate</p><p className="mt-1 text-2xl font-semibold text-sky-600">{totalRate}<span className="text-sm font-normal text-slate-400">/min</span></p></div></Card>
      </div>

      <Card>
        {loading && !data ? <Spinner /> : (data || []).length === 0 ? (
          <EmptyState title="No SMTP accounts yet" hint="Add a provider like Amazon SES, Gmail, Mailgun or Postmark."
            action={<Button onClick={() => setOpen(true)}>Add account</Button>} />
        ) : (
          <Table head={['Provider', 'From', 'Throttle', 'Rotation', 'Status', 'Actions']}>
            {(data || []).map((a) => (
              <tr key={a.id}>
                <td className="px-3 py-2.5">
                  <div className="font-medium text-slate-800">{a.host}</div>
                  <div className="text-xs text-slate-400">port {a.port} · {a.tls_mode} · {a.auth_method}</div>
                </td>
                <td className="px-3 py-2.5">
                  <div>{a.from_email}</div>
                  {a.from_name && <div className="text-xs text-slate-400">{a.from_name}</div>}
                </td>
                <td className="px-3 py-2.5 text-slate-600">{a.max_emails_per_min}/min</td>
                <td className="px-3 py-2.5 text-slate-600">w{a.rotation_weight}</td>
                <td className="px-3 py-2.5">
                  <Badge color={a.verified ? 'green' : 'amber'}>{a.verified ? 'verified' : 'unverified'}</Badge>
                </td>
                <td className="px-3 py-2.5">
                  <div className="flex items-center gap-2">
                    <input
                      value={testTarget[a.id] || ''}
                      onChange={(e) => setTestTarget({ ...testTarget, [a.id]: e.target.value })}
                      placeholder="test@inbox.com"
                      className="w-32 rounded-md border border-slate-300 px-2 py-1 text-xs"
                    />
                    <Button size="sm" variant="secondary" disabled={busy} onClick={() => test(a.id)}>Test</Button>
                    <Button size="sm" variant="danger" onClick={() => remove(a.id)}>Delete</Button>
                  </div>
                </td>
              </tr>
            ))}
          </Table>
        )}
      </Card>

      {toast && <div className="fixed bottom-4 right-4 rounded-lg bg-slate-900 px-4 py-2 text-sm text-white shadow-lg">{toast}</div>}

      <Modal open={open} title="Add SMTP account" onClose={() => setOpen(false)}
        footer={<><Button variant="secondary" onClick={() => setOpen(false)}>Cancel</Button><Button form="smtp-form" type="submit" disabled={busy}>{busy ? 'Saving…' : 'Save account'}</Button></>}>
        <form id="smtp-form" onSubmit={create} className="space-y-3">
          <div className="grid grid-cols-3 gap-3">
            <div className="col-span-2"><Input label="SMTP host" required value={form.host} onChange={set('host')} placeholder="smtp.mailgun.org" /></div>
            <Input label="Port" type="number" required value={form.port} onChange={set('port')} />
          </div>
          <div className="grid grid-cols-2 gap-3">
            <Input label="Username" required value={form.username} onChange={set('username')} />
            <Input label="Password" type="password" required value={form.password} onChange={set('password')} />
          </div>
          <div className="grid grid-cols-2 gap-3">
            <Input label="From email" required type="email" value={form.from_email} onChange={set('from_email')} />
            <Input label="From name" value={form.from_name} onChange={set('from_name')} />
          </div>
          <div className="grid grid-cols-2 gap-3">
            <Select label="TLS mode" value={form.tls_mode} onChange={set('tls_mode')} options={['starttls', 'ssl', 'none']} />
            <Select label="Auth method" value={form.auth_method} onChange={set('auth_method')} options={['login', 'plain', 'cram-md5']} />
          </div>
          <div className="grid grid-cols-2 gap-3">
            <Input label="Max emails / min" type="number" value={form.max_emails_per_min} onChange={set('max_emails_per_min')} hint="Provider limit to avoid bans" />
            <Input label="Rotation weight" type="number" value={form.rotation_weight} onChange={set('rotation_weight')} hint="Share of parallel traffic" />
          </div>
        </form>
      </Modal>
    </div>
  )
}
