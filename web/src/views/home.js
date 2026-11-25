import { api } from '../api.js'
import { esc, summary } from '../format.js'
import { icon, intro, loading, errorBlock, alertBox } from '../ui.js'

export async function renderHome(main) {
  const head = intro('Fikua Signatura', 'Inici', 'El que demana atenció.',
    `<a class="btn primary" href="#/usuaris/nou" data-testid="home-new-user">${icon('plus')}Nou usuari</a>`)
  main.innerHTML = head + loading()
  let users, creds
  try {
    ;[users, creds] = await Promise.all([api.users(), api.credentials()])
  } catch {
    main.innerHTML = head + errorBlock('No es pot llegir l\'estat', 'El servei de signatura no respon. Les firmes en curs no es veuen afectades.', '#/')
    return
  }
  const s = summary(users, creds)
  const kpi = (label, n, href, warn = false) =>
    `<div class="meta-cell${warn && n ? ' warn' : ''}"><dt>${esc(label)}</dt><dd><a class="n" href="${href}">${n}</a></dd></div>`
  main.innerHTML = head +
    `<dl class="meta-grid">${kpi('Usuaris actius', s.activeUsers, '#/usuaris')}${kpi('Credencials actives', s.activeCredentials, '#/credencials')}` +
    `${kpi('Pendents de certificat', s.pending, '#/credencials', true)}${kpi('Caduquen en 30 dies', s.expiring, '#/certificats', true)}</dl>` +
    `<div class="section-head section-gap"><h2>Cal atenció</h2></div>` +
    (s.alerts.length
      ? `<ul class="alerts">${s.alerts.map(a => `<li>${alertBox(a.tone, a.title, ' ' + a.text, a.href)}</li>`).join('')}</ul>`
      : `<p class="foot-note tight">Res pendent.</p>`)
}
