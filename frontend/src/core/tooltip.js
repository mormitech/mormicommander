// Tooltips: every element with a title (or data-tip) shows it in a small dark bubble after a second; the system's
// tooltip is not used (same as mormiView).
const DELAY = 1000;

// tipMore(path, fields) → a promise of one more line for an image's tooltip ("Picture: 1920 × 1080"); set by
// features/config/listopts.js.
let tipMore = null;
export function setTipMore(fn) { tipMore = fn; }

export function initTips() {
  const tip = document.createElement('div');
  tip.id = 'tip';
  tip.setAttribute('role', 'tooltip');
  document.body.appendChild(tip);
  let timer = 0, from = null;

  const hide = () => {
    clearTimeout(timer);
    tip.classList.remove('on');
    if (from && from.dataset.tip && from.dataset.tipFromTitle) { from.title = from.dataset.tip; delete from.dataset.tip; delete from.dataset.tipFromTitle; }
    from = null;
  };
  const show = (el) => {
    tip.textContent = el.dataset.tip;
    place(el);
  };
  const place = (el) => {
    const r = el.getBoundingClientRect(), t = tip.getBoundingClientRect();
    const x = Math.max(8, Math.min(innerWidth - t.width - 8, r.left + r.width / 2 - t.width / 2));
    const below = r.bottom + 8 + t.height < innerHeight;
    tip.style.left = `${x}px`;
    tip.style.top = `${below ? r.bottom + 8 : Math.max(8, r.top - 8 - t.height)}px`;
    tip.classList.add('on');
  };

  document.addEventListener('pointerover', (ev) => {
    const el = ev.target.closest?.('[title],[data-tip]');
    if (el === from) return;
    hide();
    if (!el || ev.pointerType === 'touch') return;
    const text = el.title || el.dataset.tip;
    if (!text) return;
    // a name whose tooltip is only for when it is cut off (Options > Tooltips) shows none when it fits
    if (el.dataset.tipCut && el.scrollWidth <= el.clientWidth) return;
    if (!el.getAttribute('aria-label')) el.setAttribute('aria-label', text);
    if (el.title) { el.dataset.tip = text; el.dataset.tipFromTitle = '1'; el.removeAttribute('title'); }
    from = el;
    const delay = el.dataset.tipDelay !== undefined ? Math.max(0, +el.dataset.tipDelay || 0) : DELAY;
    timer = setTimeout(() => {
      show(el);
      // an image in a file list adds its picture size once it is known (Options > Tooltips > per-type fields)
      const more = el.dataset.tipImg;
      // (with Tooltip fields per file type, data-tip-fields names the fields Go reads, F-821)
      if (more && tipMore) tipMore(more, el.dataset.tipFields || '').then((line) => { if (line && from === el) { tip.textContent = el.dataset.tip.trim() ? `${el.dataset.tip}\n${line}` : line; place(el); } }, () => {});
    }, delay);
  });
  document.addEventListener('pointerout', (ev) => { if (from && !from.contains(ev.relatedTarget)) hide(); });
  for (const t of ['pointerdown', 'keydown', 'wheel', 'blur']) window.addEventListener(t, hide, true);
}
