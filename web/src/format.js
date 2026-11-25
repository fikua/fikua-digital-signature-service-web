// Pure helpers of the portal: no DOM, no network, so they are tested with node --test.

const DAY = 24 * 60 * 60 * 1000

/** Text safe inside HTML, attributes included. */
export function esc(value) {
  return String(value ?? '').replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[c])
}

/** 25/11/2026, in Madrid's time. */
export function fmtDate(iso) {
  if (!iso) return ''
  return new Date(iso).toLocaleDateString('ca-ES', { timeZone: 'Europe/Madrid', day: '2-digit', month: '2-digit', year: 'numeric' })
}

export function daysUntil(iso, now = new Date()) {
  return Math.floor((new Date(iso).getTime() - now.getTime()) / DAY)
}

export const ENVIRONMENTS = [
  { value: 'dev', label: 'DEV' },
  { value: 'stg', label: 'STG' },
  { value: 'pro', label: 'PRO' },
  { value: 'local', label: 'Local' },
]

export function envLabel(env) {
  return ENVIRONMENTS.find(e => e.value === env)?.label ?? env ?? ''
}

/** The badge of a certificate: expired, expiring within 30 days, valid. */
export function certState(cert, now = new Date()) {
  if (!cert) return { label: 'Sense certificat', tone: 'warning' }
  const days = daysUntil(cert.notAfter, now)
  if (days < 0) return { label: 'Caducat', tone: 'error' }
  if (days <= 30) return { label: `Caduca en ${days} ${days === 1 ? 'dia' : 'dies'}`, tone: 'warning' }
  return { label: `Vàlid fins al ${fmtDate(cert.notAfter)}`, tone: 'success' }
}

/** The badge of a credential. */
export function credState(cred) {
  switch (cred.status) {
    case 'active': return { label: 'Activa', tone: 'success' }
    case 'pending': return { label: 'Pendent de certificat', tone: 'warning' }
    case 'revoked': return { label: 'Revocada', tone: 'error' }
    default: return { label: cred.status, tone: '' }
  }
}

export function userState(user) {
  return user.active ? { label: 'Actiu', tone: 'success' } : { label: 'Inactiu', tone: '' }
}

const CLIENT_ID = /^[a-z0-9][a-z0-9-]{2,40}$/

export function validClientId(id) {
  return CLIENT_ID.test(id ?? '')
}

function trimDashes(s) {
  let start = 0
  let end = s.length
  while (start < end && s[start] === '-') start++
  while (end > start && s[end - 1] === '-') end--
  return s.slice(start, end)
}

/** "Cooperativa Vallès", "dev" → "cooperativa-valles-dev". */
export function proposeClientId(customer, env) {
  let slug = ''
  for (const ch of String(customer ?? '').normalize('NFD').replaceAll(/\p{M}/gu, '').toLowerCase()) {
    const ok = (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9')
    if (ok) slug += ch
    else if (!slug.endsWith('-')) slug += '-'
  }
  slug = trimDashes(slug)
  if (!slug) return ''
  const suffix = env ? '-' + env : ''
  return trimDashes(slug.slice(0, 41 - suffix.length)) + suffix
}

function matches(q, ...values) {
  const needle = (q ?? '').trim().toLowerCase()
  return !needle || values.some(v => String(v ?? '').toLowerCase().includes(needle))
}

export function filterUsers(users, q, state) {
  return users.filter(u => matches(q, u.clientId, u.customer, u.description) &&
    (!state || (state === 'actiu') === u.active))
}

export function filterCredentials(creds, q, status) {
  return creds.filter(c => matches(q, c.id, c.clientId, c.certificate?.subject) && (!status || c.status === status))
}

/** The common name of a distinguished name, or the whole name. */
export function commonName(dn) {
  const m = /(?:^|,)\s*CN=((?:\\,|[^,])+)/.exec(dn ?? '')
  return m ? m[1].replaceAll(String.raw`\,`, ',').trim() : (dn ?? '')
}

/**
 * Certificates in use, by issuer: one entry per certificate (sha256) with the
 * credentials that sign with it. Revoked credentials are left out.
 */
export function certificateTree(creds) {
  const issuers = new Map()
  for (const c of creds) {
    if (!c.certificate || c.status === 'revoked') continue
    const cert = c.certificate
    if (!issuers.has(cert.issuer)) issuers.set(cert.issuer, new Map())
    const certs = issuers.get(cert.issuer)
    if (!certs.has(cert.sha256)) certs.set(cert.sha256, { ...cert, credentials: [] })
    certs.get(cert.sha256).credentials.push(c.id)
  }
  return [...issuers].map(([issuer, certs]) => ({ issuer, certificates: [...certs.values()] }))
    .sort((a, b) => commonName(a.issuer).localeCompare(commonName(b.issuer)))
}

/** The figures and the warnings of the home page. */
export function summary(users, creds, now = new Date()) {
  const live = creds.filter(c => c.status !== 'revoked')
  const expiring = live.filter(c => c.certificate && daysUntil(c.certificate.notAfter, now) <= 30)
  const pending = live.filter(c => c.status === 'pending')
  const unmanaged = users.filter(u => !u.managed)
  const alerts = [
    ...expiring.map(c => ({
      tone: daysUntil(c.certificate.notAfter, now) < 0 ? 'error' : 'warning',
      title: `${c.id}: ${certState(c.certificate, now).label.toLowerCase()}`,
      text: 'Cal un certificat nou: amb la mateixa clau és una renovació.',
      href: `#/credencial/${encodeURIComponent(c.id)}`,
    })),
    ...pending.map(c => ({
      tone: 'warning', title: `${c.id}: pendent de certificat`,
      text: 'Baixa la CSR, porta-la a la PKI i puja el certificat.',
      href: `#/credencial/${encodeURIComponent(c.id)}`,
    })),
    ...(unmanaged.length ? [{
      tone: 'info', title: `${unmanaged.length} ${unmanaged.length === 1 ? 'usuari' : 'usuaris'} de la configuració`,
      text: 'Es gestionen des del desplegament fins que s\'adopten aquí.',
      href: '#/usuaris',
    }] : []),
  ]
  return {
    activeUsers: users.filter(u => u.active).length,
    activeCredentials: creds.filter(c => c.status === 'active').length,
    pending: pending.length,
    expiring: expiring.length,
    alerts,
  }
}

export function initials(name) {
  const parts = String(name ?? '').trim().split(/\s+/).filter(Boolean)
  return ((parts[0]?.[0] ?? '') + (parts.length > 1 ? parts.at(-1)[0] : '')).toUpperCase() || 'AD'
}

/** The message of an API error, in the portal's language when it is a known one. */
export function errorText(err) {
  const known = {
    'There is a user with that id': 'Ja hi ha un usuari amb aquest client_id.',
    'There is a client with that id': 'Ja hi ha un client amb aquest client_id a Keycloak.',
    'No accepted CA is configured': 'El servei encara no té cap CA acceptada: no es pot instal·lar cap certificat.',
    'The certificate does not lead to an accepted CA': 'El certificat no ve d\'una CA acceptada.',
    'The credential is revoked': 'La credencial està revocada.',
    'A user of the configuration: adopt it first': 'És un usuari de la configuració: cal adoptar-lo primer.',
    'No certificate in PEM': 'No hi ha cap certificat al PEM.',
    'Invalid subject': 'El subject no és un nom distingit vàlid (p. ex. CN=nom,O=organització).',
  }
  if (err?.status === 503 || err?.status === 502) return 'El servei no respon ara mateix. Torna-ho a provar d\'aquí a uns minuts.'
  if (err?.status === 409) return 'Algú altre ho acaba de fer: torna a carregar la pàgina.'
  const d = err?.description ?? ''
  return known[d] ?? (d || 'No s\'ha pogut fer.')
}
