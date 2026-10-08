import { NavLink, Outlet, useNavigate } from 'react-router-dom'
import { useAuth } from '../context/AuthContext'
import { setToken } from '../lib/api'

const nav = [
  { to: '/', label: 'Dashboard', end: true },
  { to: '/campaigns', label: 'Campaigns' },
  { to: '/lists', label: 'Lists & Subscribers' },
  { to: '/smtp', label: 'SMTP Accounts' },
  { to: '/domains', label: 'Domains & DNS' },
  { to: '/analytics', label: 'Analytics' },
]

function Icon({ path }) {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" className="h-4.5 w-4.5 h-[18px] w-[18px]">
      <path d={path} strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  )
}

const icons = {
  '/': 'M3 12l9-9 9 9M5 10v10h14V10',
  '/campaigns': 'M3 8l9-5 9 5v8l-9 5-9-5V8zM3 8l9 5 9-5M12 13v8',
  '/lists': 'M4 6h16M4 12h16M4 18h10',
  '/smtp': 'M4 4h16v12H5.2L4 17.5V4zM8 9h8M8 12h5',
  '/domains': 'M12 3a9 9 0 100 18 9 9 0 000-18zM3 12h18M12 3c2.5 2.7 2.5 15.3 0 18M12 3c-2.5 2.7-2.5 15.3 0 18',
  '/analytics': 'M4 20V10M10 20V4M16 20v-7M22 20H2',
}

export default function Layout() {
  const { user, logout } = useAuth()
  const navigate = useNavigate()

  function signOut() {
    setToken(null)
    logout()
    navigate('/login')
  }

  return (
    <div className="flex min-h-screen bg-slate-50 text-slate-900">
      <aside className="hidden w-60 flex-col border-r border-slate-200 bg-white md:flex">
        <div className="flex items-center gap-2 px-5 py-4">
          <div className="grid h-8 w-8 place-items-center rounded-lg bg-slate-900 text-sm font-bold text-white">E</div>
          <span className="font-semibold">EmailSender</span>
        </div>
        <nav className="flex-1 space-y-1 px-3 py-2">
          {nav.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              end={item.end}
              className={({ isActive }) =>
                `flex items-center gap-3 rounded-lg px-3 py-2 text-sm font-medium transition ${
                  isActive ? 'bg-slate-900 text-white' : 'text-slate-600 hover:bg-slate-100'
                }`
              }
            >
              <Icon path={icons[item.to]} />
              {item.label}
            </NavLink>
          ))}
        </nav>
        <div className="border-t border-slate-100 p-3">
          <div className="px-2 py-1 text-xs text-slate-400">Signed in as</div>
          <div className="truncate px-2 text-sm font-medium text-slate-700">{user?.user_id?.slice(0, 8) || '—'}</div>
          <button onClick={signOut} className="mt-2 w-full rounded-lg border border-slate-200 px-3 py-1.5 text-xs font-medium text-slate-600 hover:bg-slate-50">
            Sign out
          </button>
        </div>
      </aside>

      <div className="flex flex-1 flex-col">
        <header className="flex items-center justify-between border-b border-slate-200 bg-white px-6 py-3 md:hidden">
          <span className="font-semibold">EmailSender</span>
          <button onClick={signOut} className="text-sm text-slate-500">Sign out</button>
        </header>
        <nav className="flex gap-1 overflow-x-auto border-b border-slate-200 bg-white px-4 py-2 md:hidden">
          {nav.map((item) => (
            <NavLink key={item.to} to={item.to} end={item.end}
              className={({ isActive }) => `whitespace-nowrap rounded-lg px-3 py-1.5 text-sm ${isActive ? 'bg-slate-900 text-white' : 'text-slate-600'}`}>
              {item.label}
            </NavLink>
          ))}
        </nav>
        <main className="mx-auto w-full max-w-6xl flex-1 px-6 py-6">
          <Outlet />
        </main>
      </div>
    </div>
  )
}
