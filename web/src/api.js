// Calls to the portal's backend: the session lives in an HttpOnly cookie, the
// CSRF token goes in a header on every change.

let csrf = ''

export class ApiError extends Error {
  constructor(status, error, description) {
    super(description || error || `HTTP ${status}`)
    this.status = status
    this.error = error
    this.description = description
  }
}

/** Who is signed in; null when nobody is. */
export async function currentSession() {
  const res = await fetch('/auth/session', { credentials: 'same-origin' })
  if (res.status === 401) return null
  if (!res.ok) throw new ApiError(res.status)
  const s = await res.json()
  csrf = s.csrf
  return s
}

export function signIn() {
  const back = location.pathname + location.hash
  location.assign('/auth/login?return=' + encodeURIComponent(back))
}

export async function signOut() {
  const res = await fetch('/auth/logout', { method: 'POST', credentials: 'same-origin', headers: { 'X-CSRF-Token': csrf } })
  const body = await res.json().catch(() => ({}))
  location.assign(body.logout || '/')
}

/**
 * One call to the administration API. A 401 means the session has ended:
 * the page goes back to the sign-in.
 */
export async function call(method, path, { json, text, type, accept = 'application/json' } = {}) {
  const headers = { Accept: accept }
  let body
  if (json !== undefined) {
    headers['Content-Type'] = 'application/json'
    body = JSON.stringify(json)
  } else if (text !== undefined) {
    headers['Content-Type'] = type || 'text/plain'
    body = text
  }
  if (method !== 'GET') headers['X-CSRF-Token'] = csrf
  const res = await fetch('/api/v1' + path, { method, headers, body, credentials: 'same-origin' })
  if (res.status === 401) {
    signIn()
    return new Promise(() => {}) // the page is leaving
  }
  const isJson = (res.headers.get('Content-Type') || '').includes('json')
  const payload = isJson ? await res.json().catch(() => null) : await res.text()
  if (!res.ok) throw new ApiError(res.status, payload?.error, payload?.error_description)
  return payload
}

export const api = {
  users: () => call('GET', '/users'),
  user: id => call('GET', `/users/${encodeURIComponent(id)}`),
  createUser: u => call('POST', '/users', { json: u }),
  adoptUser: (id, u) => call('PUT', `/users/${encodeURIComponent(id)}`, { json: u }),
  setActive: (id, active) => call('PATCH', `/users/${encodeURIComponent(id)}`, { json: { active } }),
  rotateSecret: id => call('POST', `/users/${encodeURIComponent(id)}/secret`),
  credentials: () => call('GET', '/credentials'),
  createCredential: clientId => call('POST', `/users/${encodeURIComponent(clientId)}/credentials`),
  csr: (id, subject) => call('GET', `/credentials/${encodeURIComponent(id)}/csr` + (subject ? '?subject=' + encodeURIComponent(subject) : ''),
    { accept: 'application/pkcs10, application/json' }),
  installCertificate: (id, pem) => call('PUT', `/credentials/${encodeURIComponent(id)}/certificate`, { text: pem, type: 'application/x-pem-file' }),
  rotatePassword: (id, overlapHours) => call('POST', `/credentials/${encodeURIComponent(id)}/password`, { json: { overlapHours } }),
  revoke: id => call('DELETE', `/credentials/${encodeURIComponent(id)}`),
}
