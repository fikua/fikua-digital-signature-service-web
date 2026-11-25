import { api } from '../api.js'
import { esc, fmtDate, certState, certificateTree, commonName } from '../format.js'
import { intro, badge, loading, errorBlock } from '../ui.js'

export async function renderCertificates(main) {
  const head = intro('Cadena de confiança', 'Certificats', 'Qui emet què, fins quan val i quines credencials hi signen.')
  main.innerHTML = head + loading()
  let creds
  try {
    creds = await api.credentials()
  } catch {
    main.innerHTML = head + errorBlock('No es poden llegir els certificats', 'El servei de signatura no respon.', '#/certificats')
    return
  }
  const tree = certificateTree(creds)
  main.innerHTML = head + (tree.length ? `<ul class="tree">${tree.map(i => `
    <li><div class="card tree-node ghost-node"><div class="grow"><span class="nm">${esc(commonName(i.issuer))}</span><span class="sub mono">${esc(i.issuer)}</span></div><span class="meta-chip">Emissor</span></div>
      <ul>${i.certificates.map(cert => `<li><div class="card tree-node"><div class="grow"><span class="nm">${esc(commonName(cert.subject))}</span>
        <span class="sub">Fins al ${esc(fmtDate(cert.notAfter))} · ${cert.credentials.map(id => `<a class="mono" href="#/credencial/${encodeURIComponent(id)}">${esc(id)}</a>`).join(', ')}</span></div>
        ${badge(certState(cert))}</div></li>`).join('')}</ul></li>`).join('')}</ul>`
    : `<div class="empty-state"><h2>Cap certificat</h2><p>Les credencials actives encara no en tenen.</p></div>`) +
    `<p class="foot-note">Els certificats els emet la PKI, no aquest servei. El d'una credencial es puja des de la seva fitxa.</p>`
}
