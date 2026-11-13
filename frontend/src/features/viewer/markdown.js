// A small Markdown to HTML converter for the Lister's rendered view (key 5 on a .md file, F-571). It covers what README
// files use: headings, paragraphs, emphasis, inline code, fenced and indented code, block quotes, lists (nested by
// indentation), tables, rules, links and images. All text is escaped first, so the output never carries markup from the
// file; links and images keep only their address (the viewer decides what may load). Pure: no DOM, unit-tested.

const esc = (s) => String(s).replace(/[&<>"]/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' }[c]));

// safeURL keeps web, mail and in-page addresses and drops javascript: and the like.
function safeURL(u) {
  const t = u.trim().replace(/^<|>$/g, '');
  if (/^(https?:|mailto:|#|data:image\/)/i.test(t) || !/^[a-z][a-z0-9+.-]*:/i.test(t)) return t;
  return '';
}

// inline converts the inline syntax of one block of (unescaped) text.
export function inline(src) {
  const codes = [];
  // inline code first, so nothing inside it is touched
  let s = src.replace(/(`+)([\s\S]*?[^`])\1(?!`)/g, (_, __, c) => { codes.push(c.trim() === '' ? c : c.replace(/^ (.*) $/, '$1')); return `\u0000${codes.length - 1}\u0000`; });
  s = esc(s);
  s = s.replace(/!\[([^\]]*)\]\(([^)\s]+)(?:\s+&quot;[^&]*&quot;)?\)/g, (_, alt, url) => {
    const u = safeURL(url.replace(/&amp;/g, '&'));
    return u ? `<img alt="${alt}" src="${esc(u)}">` : alt;
  });
  s = s.replace(/\[([^\]]+)\]\(([^)\s]+)(?:\s+&quot;[^&]*&quot;)?\)/g, (_, text, url) => {
    const u = safeURL(url.replace(/&amp;/g, '&'));
    return u ? `<a href="${esc(u)}">${text}</a>` : text;
  });
  s = s.replace(/&lt;((?:https?:\/\/|mailto:)[^\s&]+)&gt;/g, (_, u) => `<a href="${u}">${u}</a>`);
  s = s.replace(/(\*\*|__)(?=\S)([\s\S]*?\S)\1/g, '<strong>$2</strong>');
  s = s.replace(/(^|[^\w*])\*(?=\S)([^*]*?\S)\*(?!\*)/g, '$1<em>$2</em>');
  s = s.replace(/(^|[^\w])_(?=\S)([^_]*?\S)_(?!\w)/g, '$1<em>$2</em>');
  s = s.replace(/~~(?=\S)([\s\S]*?\S)~~/g, '<del>$1</del>');
  s = s.replace(/ {2,}\n/g, '<br>\n');
  return s.replace(/\u0000(\d+)\u0000/g, (_, i) => `<code>${esc(codes[+i])}</code>`);
}

const LIST = /^(\s*)([-*+]|\d{1,9}[.)])\s+(.*)$/;
const indentOf = (l) => l.match(/^\s*/)[0].replace(/\t/g, '    ').length;

function table(lines) {
  const cells = (l) => l.trim().replace(/^\||\|$/g, '').split(/(?<!\\)\|/).map((c) => c.trim().replace(/\\\|/g, '|'));
  const head = cells(lines[0]);
  const align = cells(lines[1]).map((c) => (/^:-+:$/.test(c) ? 'center' : /-+:$/.test(c) ? 'right' : /^:-+/.test(c) ? 'left' : ''));
  const td = (tag, c, i) => `<${tag}${align[i] ? ` style="text-align:${align[i]}"` : ''}>${inline(c)}</${tag}>`;
  let h = `<table><thead><tr>${head.map((c, i) => td('th', c, i)).join('')}</tr></thead><tbody>`;
  for (const l of lines.slice(2)) h += `<tr>${cells(l).map((c, i) => td('td', c, i)).join('')}</tr>`;
  return h + '</tbody></table>';
}

// markdownToHTML converts a whole document.
export function markdownToHTML(md) {
  const lines = String(md).replace(/\r\n?/g, '\n').split('\n');
  return blocks(lines);
}

function blocks(lines) {
  const out = [];
  let i = 0;
  const para = [];
  const flush = () => { if (para.length) { out.push(`<p>${inline(para.join('\n'))}</p>`); para.length = 0; } };
  while (i < lines.length) {
    const l = lines[i];
    let m;
    if (/^\s*$/.test(l)) { flush(); i++; continue; }
    if ((m = l.match(/^\s{0,3}(`{3,}|~{3,})\s*([\w+-]*)/))) { // fenced code
      flush();
      const fence = m[1];
      const code = [];
      i++;
      while (i < lines.length && !lines[i].trimStart().startsWith(fence)) code.push(lines[i++]);
      i++;
      out.push(`<pre><code${m[2] ? ` class="language-${esc(m[2])}"` : ''}>${esc(code.join('\n'))}</code></pre>`);
      continue;
    }
    if (!para.length && /^( {4}|\t)/.test(l)) { // indented code
      const code = [];
      while (i < lines.length && (/^( {4}|\t)/.test(lines[i]) || (/^\s*$/.test(lines[i]) && /^( {4}|\t)/.test(lines[i + 1] || '')))) code.push(lines[i++].replace(/^( {4}|\t)/, ''));
      out.push(`<pre><code>${esc(code.join('\n'))}</code></pre>`);
      continue;
    }
    if ((m = l.match(/^\s{0,3}(#{1,6})\s+(.*?)\s*#*\s*$/))) { flush(); out.push(`<h${m[1].length}>${inline(m[2])}</h${m[1].length}>`); i++; continue; }
    if (/^\s{0,3}([-*_])(\s*\1){2,}\s*$/.test(l)) { flush(); out.push('<hr>'); i++; continue; }
    if (para.length && /^\s{0,3}(=+|-+)\s*$/.test(l)) { // setext heading
      const lvl = l.trim()[0] === '=' ? 1 : 2;
      out.push(`<h${lvl}>${inline(para.join('\n'))}</h${lvl}>`);
      para.length = 0;
      i++;
      continue;
    }
    if (/^\s{0,3}>/.test(l)) {
      flush();
      const q = [];
      while (i < lines.length && /^\s{0,3}>/.test(lines[i])) q.push(lines[i++].replace(/^\s{0,3}>\s?/, ''));
      out.push(`<blockquote>${blocks(q)}</blockquote>`);
      continue;
    }
    if (l.includes('|') && i + 1 < lines.length && /^\s*\|?\s*:?-+:?\s*(\|\s*:?-+:?\s*)*\|?\s*$/.test(lines[i + 1]) && lines[i + 1].includes('-')) {
      flush();
      const t = [l, lines[i + 1]];
      i += 2;
      while (i < lines.length && lines[i].includes('|') && !/^\s*$/.test(lines[i])) t.push(lines[i++]);
      out.push(table(t));
      continue;
    }
    if ((m = l.match(LIST)) && (!para.length || indentOf(l) === 0)) {
      flush();
      const base = indentOf(l);
      const ordered = /\d/.test(m[2]);
      const items = [];
      while (i < lines.length) {
        const x = lines[i];
        const mm = x.match(LIST);
        if (mm && indentOf(x) === base && /\d/.test(mm[2]) === ordered) { items.push([mm[3]]); i++; continue; }
        if (/^\s*$/.test(x)) {
          const next = lines[i + 1] || '';
          if (indentOf(next) > base && !/^\s*$/.test(next)) { items.at(-1).push(''); i++; continue; }
          break;
        }
        if (indentOf(x) > base || (!mm && !/^\s{0,3}(#|>|```|~~~)/.test(x))) { items.at(-1).push(x.replace(new RegExp(`^\\s{0,${base + 4}}`), '')); i++; continue; }
        break;
      }
      const tag = ordered ? 'ol' : 'ul';
      const start = ordered && parseInt(m[2], 10) !== 1 ? ` start="${parseInt(m[2], 10)}"` : '';
      out.push(`<${tag}${start}>${items.map((it) => {
        const [first, ...rest] = it;
        const task = first.match(/^\[([ xX])\]\s+(.*)$/);
        const head = task ? `<input type="checkbox" disabled${task[1] !== ' ' ? ' checked' : ''}> ${inline(task[2])}` : inline(first);
        const inner = rest.some((r) => r.trim()) ? blocks(rest) : '';
        return `<li>${head}${inner}</li>`;
      }).join('')}</${tag}>`);
      continue;
    }
    para.push(l.trim());
    i++;
  }
  flush();
  return out.join('\n');
}
