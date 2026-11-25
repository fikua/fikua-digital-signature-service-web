import { api } from '../api.js'
import { esc, fmtDate, credState, certState, filterCredentials, commonName, errorText } from '../format.js'
import { $, $$, icon, badge, intro, backLink, loading, errorBlock, alertBox, confirm, toast, busy, download } from '../ui.js'
import { showSecrets } from './secrets.js'

const unavailable = () => errorBlock('No es poden llegir les credencials', 'El servei de signatura no respon.', location.hash)

export async function renderCredentials(main) {
  const head = intro('CSC v2', 'Credencials', 'Amb què signa cada usuari: una clau, el seu certificat i una contrasenya que autoritza cada firma.')
  main.innerHTML = head + loading()
  let creds
  try {
    creds = await api.credentials()
  } catch {
    main.innerHTML = head + unavailable()
    return
  }
  main.innerHTML = head + `
    <div class="toolbar"><div class="search">${icon('search')}<label class="sr-only" for="c-q">Cerca credencials</label><input id="c-q" type="search" placeholder="Cerca per credencial, usuari o subject" data-testid="creds-search"></div>
      <div class="segmented" role="group" aria-label="Estat" id="c-state"><button type="button" aria-pressed="true" data-v="">Totes</button><button type="button" aria-pressed="false" data-v="active">Actives</button><button type="button" aria-pressed="false" data-v="pending">Pendents</button><button type="button" aria-pressed="false" data-v="revoked">Revocades</button></div></div>
    <div id="c-list"></div>`
  let status = ''
  const draw = () => {
    const list = filterCredentials(creds, $('#c-q', main).value, status)
    $('#c-list', main).innerHTML = list.length ? `<div class="table-wrap"><table class="table"><caption class="sr-only">Credencials</caption>
      <thead><tr><th scope="col">Credencial</th><th scope="col">Usuari</th><th scope="col">Certificat</th><th scope="col">Estat</th></tr></thead><tbody>
      ${list.map(c => `<tr><td><a class="mono" href="#/credencial/${encodeURIComponent(c.id)}">${esc(c.id)}</a><span class="sub">${c.key === 'file' ? 'Clau de la configuració' : 'Clau creada pel servei'}</span></td>
        <td><a class="mono" href="#/usuari/${encodeURIComponent(c.clientId)}">${esc(c.clientId)}</a></td>
        <td>${c.certificate ? badge(certState(c.certificate)) : '—'}</td><td>${badge(credState(c))}</td></tr>`).join('')}
      </tbody></table></div>` : `<div class="empty-state"><h2>Cap credencial</h2><p>No n'hi ha cap amb aquest filtre.</p></div>`
  }
  $('#c-q', main).addEventListener('input', draw)
  $$('#c-state button', main).forEach(b => b.addEventListener('click', () => {
    status = b.dataset.v
    $$('#c-state button', main).forEach(x => x.setAttribute('aria-pressed', String(x === b)))
    draw()
  }))
  draw()
}

function certificateCard(cert) {
  if (!cert) return ''
  const kv = (k, v, mono = false) => `<div class="kv"><span>${esc(k)}</span><span${mono ? ' class="mono"' : ''}>${esc(v)}</span></div>`
  return `<div class="card"><div class="card-hd"><h2>Certificat</h2>${badge(certState(cert))}</div><div class="card-bd">
    ${kv('Subject', cert.subject)}${kv('Emissor', cert.issuer)}${kv('Número de sèrie', cert.serialNumber, true)}
    ${kv('Vàlid des de', fmtDate(cert.notBefore))}${kv('Vàlid fins a', fmtDate(cert.notAfter))}${kv('SHA-256', cert.sha256, true)}</div></div>`
}

function overlapText(hours) {
  if (!hours) return 'L\'anterior ja no val.'
  return `L'anterior val ${hours} ${hours === 1 ? 'hora' : 'hores'} més.`
}

export async function renderCredential(main, id) {
  const back = backLink('#/credencials', 'Credencials')
  main.innerHTML = back + loading()
  let c
  try {
    c = (await api.credentials()).find(x => x.id === id)
  } catch {
    main.innerHTML = back + unavailable()
    return
  }
  if (!c) {
    main.innerHTML = back + errorBlock('No existeix', `No hi ha cap credencial ${id}.`)
    return
  }
  const managed = c.key === 'service'
  const open = c.status !== 'revoked'
  main.innerHTML = back + `
    <div class="detail-top"><div class="status-row">${badge(credState(c))}<span class="meta-chip">${managed ? 'Clau creada pel servei' : 'Clau de la configuració'}</span></div>
      <h1 class="type-name">${esc(c.id)}</h1>
      <p class="lede">De <a class="mono" href="#/usuari/${encodeURIComponent(c.clientId)}">${esc(c.clientId)}</a>${c.passwordRotatedAt ? ` · contrasenya del ${esc(fmtDate(c.passwordRotatedAt))}` : ''}</p>
      <div class="detail-actions" id="c-actions"></div></div>
    <div id="c-cert-flow"></div>
    ${certificateCard(c.certificate)}`
  if (!managed) {
    $('#c-actions', main).innerHTML = `<p class="foot-note tight">Es gestiona des del desplegament fins que el seu usuari s'adopta.</p>`
    return
  }
  if (!open) return

  $('#c-actions', main).innerHTML = `
    <button class="btn" type="button" id="c-pw" data-testid="rotate-password">${icon('lock')}Rota la contrasenya</button>
    <button class="btn danger" type="button" id="c-revoke" data-testid="revoke">${icon('trash')}Revoca</button>`

  const renewal = c.status === 'active'
  $('#c-cert-flow', main).innerHTML = `<div class="card"><div class="card-hd"><h2>${renewal ? 'Renovar el certificat' : 'Posar-la en marxa'}</h2></div><div class="card-bd">
    <p>${renewal ? 'Amb la mateixa clau: baixa una CSR nova, porta-la a la PKI i puja el certificat que torni.' : 'La clau ja és al servei. Baixa la CSR, porta-la a la PKI i puja el certificat que torni.'}</p>
    <div class="inline-form"><div class="field"><label for="c-subject">Subject de la CSR (opcional)</label><input id="c-subject" class="mono" placeholder="CN=${esc(c.id)}" aria-describedby="h-subject" data-testid="csr-subject"><div class="help" id="h-subject">La CA decideix el subject final.</div></div>
      <button class="btn" type="button" id="c-csr" data-testid="download-csr">${icon('download')}Baixa la CSR</button></div>
    <form id="c-cert" novalidate><div class="valbox flush" aria-live="polite" id="c-val"></div>
      <div class="field"><label for="c-pem">Certificat (PEM)</label><textarea id="c-pem" class="pem" spellcheck="false" aria-describedby="h-pem" data-testid="cert-pem"></textarea><div class="help" id="h-pem">Només el certificat, amb els intermedis si n'hi ha. Mai la clau privada.</div></div>
      <div class="field"><label for="c-file">O tria el fitxer</label><input id="c-file" type="file" accept=".pem,.crt,.cer"></div>
      <div class="form-actions"><button class="btn primary" type="submit" data-testid="install-cert">${icon('upload')}Instal·la el certificat</button></div></form>
  </div></div>`

  $('#c-csr', main).addEventListener('click', async e => {
    try {
      const pem = await busy(e.currentTarget, () => api.csr(c.id, $('#c-subject', main).value.trim()))
      download(`${c.id}.csr`, pem, 'application/pkcs10')
    } catch (err) { toast(errorText(err), 'error') }
  })
  $('#c-file', main).addEventListener('change', async e => {
    const file = e.target.files[0]
    if (file) $('#c-pem', main).value = await file.text()
  })
  $('#c-cert', main).addEventListener('submit', async e => {
    e.preventDefault()
    const pem = $('#c-pem', main).value.trim()
    if (!pem.includes('-----BEGIN CERTIFICATE-----') || pem.includes('PRIVATE KEY')) {
      $('#c-val', main).innerHTML = alertBox('error', 'No és un certificat. ', 'Enganxa el PEM del certificat (-----BEGIN CERTIFICATE-----), sense cap clau.')
      return
    }
    try {
      await busy(e.submitter, () => api.installCertificate(c.id, pem))
      toast(renewal ? 'Certificat renovat' : 'Credencial activa')
      await renderCredential(main, id)
    } catch (err) {
      $('#c-val', main).innerHTML = alertBox('error', 'Certificat refusat. ', errorText(err) + ' No s\'ha canviat res.')
    }
  })
  $('#c-pw', main).addEventListener('click', async e => {
    const ok = await confirm({ title: 'Rotar la contrasenya', ok: 'Rota', iconName: 'lock',
      text: 'El certificat i la clau no canvien. La contrasenya anterior pot continuar valent un temps, perquè el client la canviï sense tall.',
      extra: `<div class="field"><label for="m-overlap">L'anterior continua valent</label><select id="m-overlap" data-testid="overlap">
        <option value="0">No: deixa de valer ara</option><option value="1">1 hora</option><option value="24" selected>24 hores</option><option value="72">3 dies</option><option value="168">7 dies</option></select></div>` })
    if (!ok) return
    const overlap = Number($('#m-overlap').value)
    try {
      const p = await busy(e.currentTarget, () => api.rotatePassword(c.id, overlap))
      showSecrets({ eyebrow: 'Contrasenya nova', title: c.id,
        lede: overlapText(overlap),
        rows: [{ label: 'Contrasenya de la credencial (PIN)', value: p.credentialPassword }], next: `#/credencial/${encodeURIComponent(c.id)}` })
    } catch (err) { toast(errorText(err), 'error') }
  })
  $('#c-revoke', main).addEventListener('click', async e => {
    const ok = await confirm({ title: `Revocar ${c.id}`, ok: 'Revoca i destrueix la clau', danger: true, iconName: 'trash',
      text: 'La clau es destrueix: no es pot desfer.' + (c.certificate ? ` Cal revocar també el certificat «${commonName(c.certificate.subject)}» a la PKI.` : '') })
    if (!ok) return
    try {
      await busy(e.currentTarget, () => api.revoke(c.id))
      toast('Credencial revocada')
      await renderCredential(main, id)
    } catch (err) { toast(errorText(err), 'error') }
  })
}
