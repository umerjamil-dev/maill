import { useCallback, useEffect, useRef, useState } from 'react'
import { api } from '../lib/api'

// useAsync fetches data from an endpoint and exposes refresh + mutation helpers.
export function useAsync(path, deps = []) {
  const [data, setData] = useState(null)
  const [error, setError] = useState(null)
  const [loading, setLoading] = useState(true)
  const mounted = useRef(true)

  useEffect(() => {
    mounted.current = true
    return () => { mounted.current = false }
  }, [])

  const run = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      const res = await api.get(path)
      if (mounted.current) setData(res)
      return res
    } catch (e) {
      if (mounted.current) setError(e.message)
      return null
    } finally {
      if (mounted.current) setLoading(false)
    }
  }, [path])

  useEffect(() => {
    run()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [path, ...deps])

  return { data, error, loading, refresh: run, setData }
}

// useToast gives a transient message string for form feedback.
export function useToast() {
  const [toast, setToast] = useState(null)
  const timer = useRef(null)
  const show = useCallback((msg, ms = 3500) => {
    setToast(msg)
    clearTimeout(timer.current)
    timer.current = setTimeout(() => setToast(null), ms)
  }, [])
  useEffect(() => () => clearTimeout(timer.current), [])
  return { toast, show }
}

export { api }
