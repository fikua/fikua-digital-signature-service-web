import { test } from 'node:test'
import assert from 'node:assert/strict'
import {
  esc, certState, credState, proposeClientId, validClientId, filterUsers, filterCredentials,
  certificateTree, summary, commonName, initials, errorText, daysUntil,
} from './format.js'

const now = new Date('2026-11-25T10:00:00Z')
const cert = (notAfter, sha256 = 'aa', issuer = 'CN=Arrel de proves,O=Fikua') =>
  ({ subject: 'CN=x', issuer, serialNumber: '1', notBefore: '2026-01-01T00:00:00Z', notAfter, sha256 })

test('esc', () => {
  assert.equal(esc(`<a href="x" onclick='y'>&</a>`), '&lt;a href=&quot;x&quot; onclick=&#39;y&#39;&gt;&amp;&lt;/a&gt;')
  assert.equal(esc(null), '')
})

test('certState', () => {
  assert.equal(certState(null, now).tone, 'warning')
  assert.deepEqual(certState(cert('2026-11-20T00:00:00Z'), now), { label: 'Caducat', tone: 'error' })
  assert.equal(certState(cert('2026-12-05T12:00:00Z'), now).label, 'Caduca en 10 dies')
  assert.equal(certState(cert('2026-11-26T12:00:00Z'), now).label, 'Caduca en 1 dia')
  assert.equal(certState(cert('2028-09-02T00:00:00Z'), now).tone, 'success')
  assert.equal(daysUntil('2026-11-26T10:00:00Z', now), 1)
})

test('credState', () => {
  assert.equal(credState({ status: 'pending' }).label, 'Pendent de certificat')
  assert.equal(credState({ status: 'revoked' }).tone, 'error')
})

test('proposeClientId and validClientId', () => {
  assert.equal(proposeClientId('Cooperativa Vallès', 'dev'), 'cooperativa-valles-dev')
  assert.equal(proposeClientId('  Àltia & Co. ', 'pro'), 'altia-co-pro')
  assert.equal(proposeClientId('', 'dev'), '')
  const long = proposeClientId('a'.repeat(60), 'local')
  assert.equal(long.length, 41)
  assert.ok(long.endsWith('-local') && validClientId(long))
  assert.ok(!validClientId('Altia-dev') && !validClientId('-abc') && !validClientId('ab') && !validClientId(undefined))
})

test('filters', () => {
  const users = [
    { clientId: 'altia-dev', customer: 'Altia', active: true },
    { clientId: 'lab-issuer', customer: 'Laboratori', description: 'Emissor', active: false },
  ]
  assert.deepEqual(filterUsers(users, 'ALTIA', '').map(u => u.clientId), ['altia-dev'])
  assert.deepEqual(filterUsers(users, 'emissor', '').map(u => u.clientId), ['lab-issuer'])
  assert.deepEqual(filterUsers(users, '', 'inactiu').map(u => u.clientId), ['lab-issuer'])
  const creds = [{ id: 'a-1', clientId: 'altia-dev', status: 'active' }, { id: 'a-2', clientId: 'altia-dev', status: 'pending' }]
  assert.deepEqual(filterCredentials(creds, 'altia', 'pending').map(c => c.id), ['a-2'])
})

test('certificateTree groups by issuer and certificate', () => {
  const tree = certificateTree([
    { id: 'a-1', status: 'active', certificate: cert('2028-01-01T00:00:00Z', 'h1') },
    { id: 'a-2', status: 'active', certificate: cert('2028-01-01T00:00:00Z', 'h1') },
    { id: 'b-1', status: 'active', certificate: cert('2028-01-01T00:00:00Z', 'h2', 'CN=Altra CA') },
    { id: 'c-1', status: 'revoked', certificate: cert('2028-01-01T00:00:00Z', 'h3') },
    { id: 'd-1', status: 'pending', certificate: null },
  ])
  assert.deepEqual(tree.map(i => commonName(i.issuer)), ['Altra CA', 'Arrel de proves'])
  assert.deepEqual(tree[1].certificates.map(c => c.credentials), [['a-1', 'a-2']])
})

test('commonName', () => {
  assert.equal(commonName('CN=Fikua\\, segell,O=Fikua'), 'Fikua, segell')
  assert.equal(commonName('O=Fikua'), 'O=Fikua')
})

test('summary', () => {
  const s = summary(
    [{ clientId: 'a', active: true, managed: true }, { clientId: 'b', active: false, managed: false }],
    [
      { id: 'a-1', status: 'active', certificate: cert('2026-12-01T00:00:00Z') },
      { id: 'a-2', status: 'pending', certificate: null },
      { id: 'a-3', status: 'revoked', certificate: cert('2026-11-01T00:00:00Z') },
    ], now)
  assert.equal(s.activeUsers, 1)
  assert.equal(s.activeCredentials, 1)
  assert.equal(s.pending, 1)
  assert.equal(s.expiring, 1)
  assert.deepEqual(s.alerts.map(a => a.tone), ['warning', 'warning', 'info'])
  assert.equal(s.alerts[0].href, '#/credencial/a-1')
})

test('initials and errorText', () => {
  assert.equal(initials('Ada Byron Lovelace'), 'AL')
  assert.equal(initials(''), 'AD')
  assert.equal(errorText({ status: 400, description: 'Invalid subject' }).startsWith('El subject'), true)
  assert.equal(errorText({ status: 503 }).startsWith('El servei'), true)
  assert.equal(errorText({ status: 400, description: 'Something new' }), 'Something new')
})
