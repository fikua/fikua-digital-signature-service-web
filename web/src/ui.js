// DOM helpers shared by the views: icons, badges, modal, toasts, page states.
import { esc } from './format.js'

export const $ = (sel, root = document) => root.querySelector(sel)
export const $$ = (sel, root = document) => [...root.querySelectorAll(sel)]

export const icon = (name, cls = 'i') => `<svg class="${cls}" aria-hidden="true"><use href="#i-${name}"/></svg>`

export const badge = ({ label, tone }) => `<span class="badge${tone ? ' badge--' + tone : ''}">${esc(label)}</span>`

export function intro(eyebrow, title, lede, action = '') {
  return `<div class="intro"><div class="intro-text"><span class="eyebrow">${esc(eyebrow)}</span><h1>${esc(title)}</h1>` +
    (lede ? `<p>${esc(lede)}</p>` : '') + `</div>${action}</div>`
}

export const backLink = (href, label) => `<a class="back-link" href="${esc(href)}">${icon('chevron-left')}${esc(label)}</a>`

export function loading() {
  return `<div class="state-block" role="status"><span class="sr-only">Carregant</span><div class="skeleton-line"></div><div class="skeleton-line short"></div><div class="skeleton-line"></div></div>`
}

export function errorBlock(title, text, retry = '') {
  return `<div class="state-block error" role="alert"><span class="eyebrow">Error</span><h2>${esc(title)}</h2><p>${esc(text)}</p>` +
    (retry ? `<a class="btn" href="${esc(retry)}">Torna-ho a provar</a>` : '') + `</div>`
}

export function alertBox(tone, title, text, href = '') {
  const ic = { error: 'x-circle', success: 'check-circle', warning: 'alert', info: 'info' }[tone] || 'info'
  const inner = `${icon(ic)}<div><strong>${esc(title)}</strong>${esc(text)}</div>`
  if (href) return `<a class="alert alert--${tone}" href="${esc(href)}">${inner}</a>`
  return `<div class="alert alert--${tone}" role="${tone === 'error' ? 'alert' : 'status'}">${inner}</div>`
}

export function toast(text, tone = 'success') {
  const el = document.createElement('div')
  el.className = `toast toast--${tone}`
  el.setAttribute('role', 'status')
  el.textContent = text
  $('#toasts').append(el)
  setTimeout(() => el.remove(), 5000)
}

/**
 * A confirmation. Resolves with true on OK. `extra` is HTML put under the text
 * (e.g. a field); `danger` paints the button red.
 */
export function confirm({ title, text, ok, danger = false, extra = '', iconName = 'alert' }) {
  const modal = $('#modal')
  const okBtn = $('#m-ok')
  $('#m-title').textContent = title
  $('#m-text').textContent = text
  $('#m-extra').innerHTML = extra
  $('#m-icon').setAttribute('href', '#i-' + iconName)
  okBtn.textContent = ok
  okBtn.className = 'btn ' + (danger ? 'danger' : 'primary')
  const opener = document.activeElement
  modal.hidden = false
  okBtn.focus()
  return new Promise(resolve => {
    const close = answer => {
      modal.hidden = true
      okBtn.onclick = $('#m-cancel').onclick = $('#m-x').onclick = null
      document.removeEventListener('keydown', onKey)
      opener?.focus?.()
      resolve(answer)
    }
    const onKey = e => { if (e.key === 'Escape') close(false) }
    document.addEventListener('keydown', onKey)
    okBtn.onclick = () => close(true)
    $('#m-cancel').onclick = $('#m-x').onclick = () => close(false)
  })
}

/** Runs a change with the button busy, so it is not sent twice. */
export async function busy(button, work) {
  button.disabled = true
  button.setAttribute('aria-busy', 'true')
  try {
    return await work()
  } finally {
    button.disabled = false
    button.removeAttribute('aria-busy')
  }
}

/** Offers a text as a file to save. */
export function download(name, text, type) {
  const url = URL.createObjectURL(new Blob([text], { type }))
  const a = document.createElement('a')
  a.href = url
  a.download = name
  document.body.append(a)
  a.click()
  a.remove()
  setTimeout(() => URL.revokeObjectURL(url), 1000)
}
