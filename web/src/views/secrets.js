// Secrets are shown once, right after they are created, and kept only in memory.
import { esc } from '../format.js'
import { $, $$, icon, intro, alertBox, toast } from '../ui.js'

let shown = null

/**
 * Shows the secrets of one operation. rows: [{label, value, hint}];
 * then goes to `next`.
 */
export function showSecrets({ eyebrow, title, lede, rows, next }) {
  shown = { eyebrow, title, lede, rows, next }
  location.hash = '#/secrets'
}

function secretRow(r, i) {
  const hint = r.hint ? `<span class="hint">${esc(r.hint)}</span>` : ''
  return `<div class="secret-row"><span class="lbl">${esc(r.label)}</span>` +
    `<div class="copy-row"><span class="value" id="secret-${i}">${esc(r.value)}</span>` +
    `<button type="button" data-copy="${i}" aria-label="Copia ${esc(r.label)}" data-testid="copy-${i}">${icon('copy')}</button></div>${hint}</div>`
}

export function renderSecrets(main) {
  if (!shown) {
    location.replace('#/')
    return
  }
  const { eyebrow, title, lede, rows, next } = shown
  main.innerHTML = intro(eyebrow, title, lede) +
    alertBox('warning', 'Només es mostren ara', ' Desa\'ls en un gestor de contrasenyes. Si es perden, es roten: no es poden recuperar.') +
    '<div class="card"><div class="card-bd">' + rows.map(secretRow).join('') + '</div></div>' +
    `
    <label class="checkline"><input type="checkbox" id="s-ok" data-testid="secrets-saved">Els he desat</label>
    <div class="form-actions"><button class="btn primary" type="button" id="s-done" disabled data-testid="secrets-done">Continua</button></div>`
  $$('[data-copy]', main).forEach(b => b.addEventListener('click', async () => {
    await navigator.clipboard.writeText(rows[Number(b.dataset.copy)].value)
    toast('Copiat')
  }))
  $('#s-ok', main).addEventListener('change', e => { $('#s-done', main).disabled = !e.target.checked })
  $('#s-done', main).addEventListener('click', () => {
    shown = null
    location.replace(next)
  })
}

/** Leaving the page forgets them. */
export function forgetSecrets() {
  shown = null
}
