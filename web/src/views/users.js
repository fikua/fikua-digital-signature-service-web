import { api } from '../api.js'
import {
  esc, envLabel, userState, credState, certState, filterUsers, proposeClientId, validClientId, errorText, ENVIRONMENTS,
} from '../format.js'
import { $, $$, icon, badge, intro, backLink, loading, errorBlock, alertBox, confirm, toast, busy } from '../ui.js'
import { showSecrets } from './secrets.js'

const unavailable = () => errorBlock('No es poden llegir els usuaris', 'El servei de signatura no respon. Les firmes en curs no es veuen afectades: els tokens ja emesos continuen valent.', location.hash)

export async function renderUsers(main) {
  const head = intro('Keycloak · realm dss', 'Usuaris', 'Cada usuari és una aplicació que demana firmes: un per client i entorn.',
    `<a class="btn primary" href="#/usuaris/nou" data-testid="users-new">${icon('plus')}Nou usuari</a>`)
  main.innerHTML = head + loading()
  let users
  try {
    users = await api.users()
  } catch {
    main.innerHTML = head + unavailable()
    return
  }
  main.innerHTML = head + `
    <div class="toolbar">
      <div class="search">${icon('search')}<label class="sr-only" for="u-q">Cerca usuaris</label><input id="u-q" type="search" placeholder="Cerca per client_id o client" data-testid="users-search"></div>
      <div class="segmented" role="group" aria-label="Estat" id="u-state"><button type="button" aria-pressed="true" data-v="">Tots</button><button type="button" aria-pressed="false" data-v="actiu">Actius</button><button type="button" aria-pressed="false" data-v="inactiu">Inactius</button></div>
    </div><div id="u-list"></div>`
  let state = ''
  const draw = () => {
    const list = filterUsers(users, $('#u-q', main).value, state)
    $('#u-list', main).innerHTML = list.length ? `<div class="table-wrap"><table class="table"><caption class="sr-only">Usuaris</caption>
      <thead><tr><th scope="col">Usuari</th><th scope="col">Entorn</th><th scope="col" class="hide-sm">Credencials</th><th scope="col">Estat</th></tr></thead><tbody>
      ${list.map(u => `<tr><td><a href="#/usuari/${encodeURIComponent(u.clientId)}" class="mono">${esc(u.clientId)}</a><span class="sub">${esc([u.customer, u.managed ? '' : 'de la configuració'].filter(Boolean).join(' · '))}</span></td>
        <td>${esc(envLabel(u.environment) || '—')}</td><td class="num hide-sm">${u.credentials.filter(c => c.status !== 'revoked').length}</td><td>${badge(userState(u))}</td></tr>`).join('')}
      </tbody></table></div>` : `<div class="empty-state"><h2>Cap usuari</h2><p>No n'hi ha cap amb aquest filtre.</p></div>`
  }
  $('#u-q', main).addEventListener('input', draw)
  $$('#u-state button', main).forEach(b => b.addEventListener('click', () => {
    state = b.dataset.v
    $$('#u-state button', main).forEach(x => x.setAttribute('aria-pressed', String(x === b)))
    draw()
  }))
  draw()
}

function userFields(prefix, { customer = '', environment = 'dev', clientId = '', description = '' } = {}, withId = true) {
  return `<div class="grid2">
    <div class="field" id="f-${prefix}-customer"><label for="${prefix}-customer">Client</label><input id="${prefix}-customer" value="${esc(customer)}" autocomplete="off" aria-describedby="h-${prefix}-customer" data-testid="${prefix}-customer"><div class="help" id="h-${prefix}-customer">Qui farà servir l'usuari.</div></div>
    <div class="field"><label for="${prefix}-env">Entorn</label><select id="${prefix}-env" data-testid="${prefix}-env">${ENVIRONMENTS.map(e => `<option value="${e.value}"${e.value === environment ? ' selected' : ''}>${e.label}</option>`).join('')}</select></div>
    ${withId ? `<div class="field" id="f-${prefix}-id"><label for="${prefix}-id">client_id</label><input id="${prefix}-id" class="mono" value="${esc(clientId)}" autocomplete="off" aria-describedby="h-${prefix}-id" data-testid="${prefix}-id"><div class="help" id="h-${prefix}-id">Es proposa a partir del client i l'entorn. No es pot canviar després.</div></div>` : ''}
    <div class="field"><label for="${prefix}-desc">Per a què serveix (opcional)</label><input id="${prefix}-desc" value="${esc(description)}" maxlength="200" data-testid="${prefix}-desc"></div>
  </div>`
}

function markInvalid(main, field, input, message, helpId) {
  $(field, main).classList.add('err')
  $(input, main).setAttribute('aria-invalid', 'true')
  $(helpId, main).textContent = message
}

export function renderNewUser(main) {
  main.innerHTML = backLink('#/usuaris', 'Usuaris') +
    intro('Alta', 'Nou usuari', 'Es crea l\'accés a Keycloak i una credencial amb la seva clau. Podrà signar quan la credencial tingui certificat.') +
    `<div class="card"><div class="card-bd"><form id="nu-form" novalidate><div class="valbox flush" aria-live="polite" id="nu-val"></div>
      ${userFields('nu')}
      <div class="form-actions"><a class="btn" href="#/usuaris">Cancel·la</a><button class="btn primary" type="submit" data-testid="nu-submit">Crea l'usuari</button></div>
    </form></div></div>`
  let touched = false
  const propose = () => { if (!touched) $('#nu-id', main).value = proposeClientId($('#nu-customer', main).value, $('#nu-env', main).value) }
  $('#nu-customer', main).addEventListener('input', propose)
  $('#nu-env', main).addEventListener('change', propose)
  $('#nu-id', main).addEventListener('input', () => { touched = true })
  $('#nu-form', main).addEventListener('submit', async e => {
    e.preventDefault()
    $$('.field.err', main).forEach(f => f.classList.remove('err'))
    $$('[aria-invalid]', main).forEach(f => f.removeAttribute('aria-invalid'))
    $('#h-nu-customer', main).textContent = 'Qui farà servir l\'usuari.'
    $('#h-nu-id', main).textContent = 'Es proposa a partir del client i l\'entorn. No es pot canviar després.'
    const user = {
      clientId: $('#nu-id', main).value.trim(), customer: $('#nu-customer', main).value.trim(),
      environment: $('#nu-env', main).value, description: $('#nu-desc', main).value.trim(),
    }
    let ok = true
    if (!user.customer) { markInvalid(main, '#f-nu-customer', '#nu-customer', 'Escriu el client.', '#h-nu-customer'); ok = false }
    if (!validClientId(user.clientId)) { markInvalid(main, '#f-nu-id', '#nu-id', 'De 3 a 41 caràcters: minúscules, xifres i guions, començant per lletra o xifra.', '#h-nu-id'); ok = false }
    if (!ok) return
    try {
      const created = await busy(e.submitter, () => api.createUser(user))
      showSecrets({
        eyebrow: 'Usuari creat', title: created.clientId,
        lede: 'Dona aquests valors a qui integra el client. La credencial queda pendent de certificat.',
        rows: [
          { label: 'client_id', value: created.clientId },
          { label: 'client_secret', value: created.clientSecret, hint: 'Per demanar tokens a https://dss.fikua.com/oauth2/token.' },
          { label: 'credentialID', value: created.credentialId },
          { label: 'Contrasenya de la credencial (PIN)', value: created.credentialPassword, hint: 'Autoritza cada firma.' },
        ],
        next: `#/credencial/${encodeURIComponent(created.credentialId)}`,
      })
    } catch (err) {
      $('#nu-val', main).innerHTML = alertBox('error', 'No s\'ha creat l\'usuari. ', errorText(err) + ' No s\'ha creat res.')
    }
  })
}

export async function renderUser(main, clientId) {
  const back = backLink('#/usuaris', 'Usuaris')
  main.innerHTML = back + loading()
  let u
  try {
    u = await api.user(clientId)
  } catch (err) {
    main.innerHTML = back + (err.status === 404 ? errorBlock('No existeix', `No hi ha cap usuari ${clientId}.`) : unavailable())
    return
  }
  const live = u.credentials.filter(c => c.status !== 'revoked')
  main.innerHTML = back + `
    <div class="detail-top"><div class="status-row">${badge(userState(u))}${u.managed ? '' : badge({ label: 'De la configuració', tone: 'info' })}<span class="meta-chip">${esc(envLabel(u.environment))}</span></div>
      <h1 class="type-name">${esc(u.clientId)}</h1><p class="lede">${esc(u.customer || '')}${u.description ? ' · ' + esc(u.description) : ''}</p>
      <div class="detail-actions" id="u-actions"></div></div>
    <div id="u-adopt"></div>
    <div class="section-head"><h2>Credencials</h2></div>
    <div class="table-wrap"><table class="table"><caption class="sr-only">Credencials de ${esc(u.clientId)}</caption>
      <thead><tr><th scope="col">Credencial</th><th scope="col">Certificat</th><th scope="col">Estat</th></tr></thead><tbody>
      ${u.credentials.map(c => `<tr><td><a class="mono" href="#/credencial/${encodeURIComponent(c.id)}">${esc(c.id)}</a></td>
        <td>${c.certificate ? badge(certState(c.certificate)) : '—'}</td><td>${badge(credState(c))}</td></tr>`).join('') ||
        '<tr><td colspan="3">Cap credencial.</td></tr>'}
      </tbody></table></div>`

  const actions = $('#u-actions', main)
  if (!u.managed) {
    $('#u-adopt', main).innerHTML = `<div class="card"><div class="card-bd">
      ${alertBox('info', 'Usuari de la configuració. ', 'Es gestiona des del desplegament. Per gestionar-lo aquí, adopta\'l: les seves credencials no canvien.')}
      <form id="ad-form" novalidate><div class="valbox flush" aria-live="polite" id="ad-val"></div>${userFields('ad', { environment: u.environment || 'dev' }, false)}
      <div class="form-actions"><button class="btn primary" type="submit" data-testid="adopt">Adopta</button></div></form></div></div>`
    $('#ad-form', main).addEventListener('submit', async e => {
      e.preventDefault()
      const customer = $('#ad-customer', main).value.trim()
      if (!customer) { markInvalid(main, '#f-ad-customer', '#ad-customer', 'Escriu el client.', '#h-ad-customer'); return }
      try {
        await busy(e.submitter, () => api.adoptUser(u.clientId, { customer, environment: $('#ad-env', main).value, description: $('#ad-desc', main).value.trim() }))
        toast('Usuari adoptat')
        await renderUser(main, clientId)
      } catch (err) {
        $('#ad-val', main).innerHTML = alertBox('error', 'No s\'ha adoptat. ', errorText(err))
      }
    })
    return
  }

  actions.innerHTML = `
    <button class="btn" type="button" id="u-rotate" data-testid="rotate-secret">${icon('lock')}Rota el secret</button>
    <button class="btn" type="button" id="u-cred" data-testid="new-credential"${u.active ? '' : ' disabled'}>${icon('plus')}Nova credencial</button>
    <button class="btn ${u.active ? 'danger' : ''}" type="button" id="u-active" data-testid="toggle-active">${u.active ? 'Desactiva' : 'Activa'}</button>`

  $('#u-rotate', main).addEventListener('click', async e => {
    const ok = await confirm({ title: 'Rotar el secret', ok: 'Rota', iconName: 'lock',
      text: `El secret actual de ${u.clientId} deixa de valer ara mateix; els tokens ja emesos duren fins a una hora. Cal canviar-lo al client alhora.` })
    if (!ok) return
    try {
      const s = await busy(e.currentTarget, () => api.rotateSecret(u.clientId))
      showSecrets({ eyebrow: 'Secret nou', title: u.clientId, lede: 'L\'anterior ja no val.',
        rows: [{ label: 'client_secret', value: s.clientSecret }], next: `#/usuari/${encodeURIComponent(u.clientId)}` })
    } catch (err) { toast(errorText(err), 'error') }
  })
  $('#u-cred', main).addEventListener('click', async e => {
    const ok = await confirm({ title: 'Nova credencial', ok: 'Crea-la', iconName: 'plus',
      text: `Fikua Signatura crearà una clau nova per a ${u.clientId}. Quedarà pendent de certificat.` })
    if (!ok) return
    try {
      const c = await busy(e.currentTarget, () => api.createCredential(u.clientId))
      showSecrets({ eyebrow: 'Credencial creada', title: c.credentialId, lede: 'Pendent de certificat.',
        rows: [{ label: 'credentialID', value: c.credentialId }, { label: 'Contrasenya de la credencial (PIN)', value: c.credentialPassword }],
        next: `#/credencial/${encodeURIComponent(c.credentialId)}` })
    } catch (err) { toast(errorText(err), 'error') }
  })
  $('#u-active', main).addEventListener('click', async e => {
    if (u.active) {
      const ok = await confirm({ title: `Desactivar ${u.clientId}`, ok: 'Desactiva', danger: true,
        text: `Deixa de signar ara mateix, també amb els tokens que ja té (${live.length} ${live.length === 1 ? 'credencial' : 'credencials'}). Es pot tornar a activar.` })
      if (!ok) return
    }
    try {
      await busy(e.currentTarget, () => api.setActive(u.clientId, !u.active))
      toast(u.active ? 'Usuari desactivat' : 'Usuari activat')
      await renderUser(main, clientId)
    } catch (err) { toast(errorText(err), 'error') }
  })
}
