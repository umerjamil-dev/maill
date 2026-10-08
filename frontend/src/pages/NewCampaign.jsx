import { useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { useAsync, api, useToast } from '../lib/hooks'
import { Card, Button, Input, Textarea, Select, EmptyState } from '../components/ui'

const STARTER_HTML = `<h1 style="font-family:sans-serif">Hi {{name}},</h1>
<p style="font-family:sans-serif;color:#334155">
  You're receiving this because you subscribed to our list. Here's what's new...
</p>
<p style="font-family:sans-serif;color:#334155">
  <a href="{{unsubscribe_url}}" style="color:#94a3b8;font-size:12px">Unsubscribe</a>
</p>`

const mergeTags = [
  { token: '{{name}}', label: 'First/last name' },
  { token: '{{email}}', label: 'Subscriber email' },
  { token: '{{unsubscribe_url}}', label: 'One-click unsubscribe' },
]

export default function NewCampaign() {
  const lists = useAsync('/lists')
  const smtp = useAsync('/smtp-accounts')
  const navigate = useNavigate()
  const { toast, show } = useToast()

  const [form, setForm] = useState({
    list_id: '', smtp_account_id: '', subject: '', preheader: '', html_body: STARTER_HTML,
  })
  const [testTo, setTestTo] = useState('')
  const [saving, setSaving] = useState(false)
  const [testing, setTesting] = useState(false)
  const [createdId, setCreatedId] = useState(null)

  function set(k) {
    return (e) => setForm({ ...form, [k]: e.target.value })
  }

  function insertTag(token) {
    const el = textareaEl
    if (!el) { setForm({ ...form, html_body: form.html_body + token }); return }
    const start = el.selectionStart ?? el.value.length
    const end = el.selectionEnd ?? start
    const next = el.value.slice(0, start) + token + el.value.slice(end)
    setForm({ ...form, html_body: next })
    requestAnimationFrame(() => {
      el.focus()
      el.selectionStart = el.selectionEnd = start + token.length
    })
  }

  async function saveDraft() {
    if (!form.list_id || !form.smtp_account_id || !form.subject || !form.html_body) {
      show('Fill in list, SMTP account, subject and body first')
      return null
    }
    const res = await api.post('/campaigns', form)
    setCreatedId(res.id)
    show('Draft saved')
    return res.id
  }

  async function onTest() {
    if (!testTo) return show('Enter a test recipient email')
    setTesting(true)
    try {
      const id = createdId || (await saveDraft())
      if (!id) return
      await api.post(`/campaigns/${id}/test`, { to: testTo })
      show(`Test email sent to ${testTo}`)
    } catch (err) {
      show(`Test failed: ${err.message}`)
    } finally {
      setTesting(false)
    }
  }

  async function onDispatch() {
    setSaving(true)
    try {
      const id = createdId || (await saveDraft())
      if (!id) return
      await api.post(`/campaigns/${id}/dispatch`, {})
      show('Campaign launched!')
      navigate(`/campaigns/${id}`)
    } catch (err) {
      show(err.message)
    } finally {
      setSaving(false)
    }
  }

  const noPrereqs = !lists.loading && !smtp.loading &&
    (lists.data || []).length === 0 && (smtp.data || []).length === 0

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold text-slate-800">New campaign</h1>
          <p className="text-sm text-slate-500">Pick your audience and sender, craft the message, test, then launch.</p>
        </div>
        <Link to="/campaigns"><Button variant="secondary" size="sm">Back</Button></Link>
      </div>

      {noPrereqs && (
        <Card>
          <EmptyState title="Set up the basics first"
            hint="You need at least one list, one subscriber and an SMTP account before launching."
            action={<div className="flex gap-2"><Link to="/lists"><Button variant="secondary" size="sm">Add list</Button></Link><Link to="/smtp"><Button size="sm">Add SMTP</Button></Link></div>} />
        </Card>
      )}

      <div className="grid gap-4 lg:grid-cols-[1fr_20rem]">
        <div className="space-y-4">
          <Card title="Audience & sender">
            <div className="grid gap-3 sm:grid-cols-2">
              <Select label="Send to list" value={form.list_id} onChange={set('list_id')}
                options={[{ value: '', label: 'Select a list…' }, ...(lists.data || []).map((l) => ({ value: l.id, label: `${l.name} (${l.subscriber_count})` }))]} />
              <Select label="Sending account" value={form.smtp_account_id} onChange={set('smtp_account_id')}
                options={[{ value: '', label: 'Select SMTP account…' }, ...(smtp.data || []).map((a) => ({ value: a.id, label: `${a.from_email} · ${a.host}` }))]} />
            </div>
          </Card>

          <Card title="Message">
            <div className="space-y-3">
              <Input label="Subject" value={form.subject} onChange={set('subject')} placeholder="Your monthly update is here" />
              <Input label="Preheader" value={form.preheader} onChange={set('preheader')} placeholder="Preview text shown after the subject" />
              <div>
                <div className="mb-1 flex items-center justify-between">
                  <span className="text-xs font-medium text-slate-600">HTML body</span>
                  <div className="flex gap-1">
                    {mergeTags.map((t) => (
                      <button key={t.token} type="button" onClick={() => insertTag(t.token)} title={t.label}
                        className="rounded border border-slate-200 bg-slate-50 px-1.5 py-0.5 font-mono text-[10px] text-slate-600 hover:bg-slate-100">
                        {t.token}
                      </button>
                    ))}
                  </div>
                </div>
                <textarea
                  ref={(el) => (textareaEl = el)}
                  rows={14}
                  value={form.html_body}
                  onChange={set('html_body')}
                  className="w-full rounded-lg border border-slate-300 px-3 py-2 font-mono text-xs text-slate-800 outline-none focus:border-slate-500 focus:ring-2 focus:ring-slate-200"
                />
              </div>
            </div>
          </Card>
        </div>

        <Card title="Review & launch" className="self-start">
          <div className="space-y-3">
            <Input label="Send test to" type="email" value={testTo} onChange={(e) => setTestTo(e.target.value)} placeholder="you@inbox.com" />
            <Button variant="secondary" className="w-full" disabled={testing} onClick={onTest}>
              {testing ? 'Sending test…' : 'Send test email'}
            </Button>
            <div className="my-2 border-t border-slate-100" />
            <Button className="w-full" disabled={saving} onClick={onDispatch}>
              {saving ? 'Launching…' : 'Save & launch'}
            </Button>
            <p className="text-xs text-slate-400">
              Launching queues the campaign. Workers send in parallel across your SMTP throttle limits and update progress live.
            </p>
          </div>
        </Card>
      </div>

      {toast && <div className="fixed bottom-4 right-4 rounded-lg bg-slate-900 px-4 py-2 text-sm text-white shadow-lg">{toast}</div>}
    </div>
  )
}

// module-scoped ref target for the HTML textarea
let textareaEl = null
