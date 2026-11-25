import '@fontsource/inter/400.css'
import '@fontsource/inter/600.css'
import '@fontsource/inter/700.css'
import '@fontsource/jetbrains-mono/400.css'
import '@fontsource/jetbrains-mono/500.css'
import '@fontsource/jetbrains-mono/600.css'
import './styles/tokens.css'
import './styles/base.css'
import './styles/app.css'

import { currentSession, signIn, signOut, call } from './api.js'
import { initials } from './format.js'
import { $, $$, errorBlock } from './ui.js'
import { renderHome } from './views/home.js'
import { renderUsers, renderNewUser, renderUser } from './views/users.js'
import { renderCredentials, renderCredential } from './views/credentials.js'
import { renderCertificates } from './views/certificates.js'
import { renderSecrets, forgetSecrets } from './views/secrets.js'

const main = $('#main')

function noAccess() {
  main.innerHTML = errorBlock('El teu compte no pot administrar Fikua Signatura',
    'Cal ser al grup dss-admins del realm fikua. Demana-ho a l\'administració de la plataforma.')
}

const routes = [
  [/^$/, 'index', renderHome],
  [/^usuaris$/, 'usuaris', renderUsers],
  [/^usuaris\/nou$/, 'usuaris', renderNewUser],
  [/^usuari\/(.+)$/, 'usuaris', (m, id) => renderUser(m, id)],
  [/^credencials$/, 'credencials', renderCredentials],
  [/^credencial\/(.+)$/, 'credencials', (m, id) => renderCredential(m, id)],
  [/^certificats$/, 'certificats', renderCertificates],
  [/^secrets$/, '', renderSecrets],
  [/^sense-acces$/, '', noAccess],
]

function route() {
  const path = decodeURIComponent(location.hash.replace(/^#\/?/, ''))
  if (path !== 'secrets') forgetSecrets()
  const [, nav, render, ...args] = (() => {
    for (const [re, n, r] of routes) {
      const m = re.exec(path)
      if (m) return [m, n, r, ...m.slice(1)]
    }
    return [null, '', m => { m.innerHTML = errorBlock('No existeix', 'Aquesta pàgina no existeix.', '#/') }]
  })()
  $$('[data-nav]').forEach(a => {
    if (a.dataset.nav === nav) a.setAttribute('aria-current', 'page')
    else a.removeAttribute('aria-current')
  })
  render(main, ...args)
  main.focus({ preventScroll: true })
  window.scrollTo(0, 0)
}

function userMenu(name) {
  const btn = $('#user-btn')
  const dd = $('#user-dd')
  $('#user-name').textContent = $('#user-name-lg').textContent = name || 'Administració'
  $('#user-initials').textContent = $('#user-initials-lg').textContent = initials(name)
  btn.addEventListener('click', () => {
    dd.hidden = !dd.hidden
    btn.setAttribute('aria-expanded', String(!dd.hidden))
  })
  document.addEventListener('click', e => {
    if (!e.target.closest('.user-menu')) {
      dd.hidden = true
      btn.setAttribute('aria-expanded', 'false')
    }
  })
  $('#logout').addEventListener('click', signOut)
}

async function start() {
  let session
  try {
    session = await currentSession()
  } catch {
    main.innerHTML = errorBlock('El portal no respon', 'Torna-ho a provar d\'aquí a uns minuts.', '#/')
    return
  }
  if (!session) {
    signIn()
    return
  }
  try {
    await call('GET', '/me')
  } catch (err) {
    if (err.status === 403) {
      noAccess()
      return
    }
  }
  $('#header-right').hidden = false
  userMenu(session.name)
  addEventListener('hashchange', route)
  route()
}

await start()
