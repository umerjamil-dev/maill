import { useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { useAuth } from '../context/AuthContext'
import { Button, Input } from '../components/ui'

export default function Register() {
  const { register } = useAuth()
  const navigate = useNavigate()
  const [form, setForm] = useState({ company: '', full_name: '', email: '', password: '' })
  const [error, setError] = useState(null)
  const [busy, setBusy] = useState(false)

  function set(k) {
    return (e) => setForm({ ...form, [k]: e.target.value })
  }

  async function submit(e) {
    e.preventDefault()
    setBusy(true)
    setError(null)
    try {
      await register(form.company, form.email, form.password, form.full_name)
      navigate('/')
    } catch (err) {
      setError(err.message)
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="grid min-h-screen place-items-center bg-slate-50 px-4">
      <div className="w-full max-w-sm">
        <div className="mb-8 flex flex-col items-center">
          <div className="grid h-11 w-11 place-items-center rounded-xl bg-slate-900 text-lg font-bold text-white">E</div>
          <h1 className="mt-3 text-xl font-semibold text-slate-800">Create your workspace</h1>
          <p className="mt-1 text-sm text-slate-500">Start sending worldwide in minutes.</p>
        </div>

        <form onSubmit={submit} className="space-y-4 rounded-xl border border-slate-200 bg-white p-6">
          {error && <div className="rounded-lg bg-rose-50 px-3 py-2 text-sm text-rose-600">{error}</div>}
          <Input label="Company / Workspace" required value={form.company} onChange={set('company')} placeholder="Acme Inc." />
          <Input label="Your name" value={form.full_name} onChange={set('full_name')} placeholder="Jane Doe" />
          <Input label="Email" type="email" required value={form.email} onChange={set('email')} placeholder="you@company.com" />
          <Input label="Password" type="password" required minLength={8} value={form.password} onChange={set('password')} hint="At least 8 characters" />
          <Button type="submit" className="w-full" disabled={busy}>{busy ? 'Creating…' : 'Create account'}</Button>
          <p className="text-center text-sm text-slate-500">
            Already have one? <Link to="/login" className="font-medium text-slate-800 hover:underline">Sign in</Link>
          </p>
        </form>
      </div>
    </div>
  )
}
