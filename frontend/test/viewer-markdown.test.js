import { describe, it, expect } from 'vitest';
import { markdownToHTML, inline } from '../src/features/viewer/markdown.js';

describe('markdownToHTML (Lister rendered view, F-571)', () => {
  it('converts headings, paragraphs and emphasis', () => {
    const h = markdownToHTML('# Title\n\nSome *em* and **strong** and `code`.\n\nSecond\nline');
    expect(h).toContain('<h1>Title</h1>');
    expect(h).toContain('<p>Some <em>em</em> and <strong>strong</strong> and <code>code</code>.</p>');
    expect(h).toContain('<p>Second\nline</p>');
  });

  it('escapes markup from the file', () => {
    const h = markdownToHTML('<script>alert(1)</script>\n\n`<b>`');
    expect(h).not.toContain('<script>');
    expect(h).toContain('&lt;script&gt;');
    expect(h).toContain('<code>&lt;b&gt;</code>');
  });

  it('drops javascript: links but keeps web links', () => {
    expect(inline('[a](javascript:alert(1))')).not.toContain('href');
    expect(inline('[a](https://example.com/x?a=1&b=2)')).toContain('href="https://example.com/x?a=1&amp;b=2"');
  });

  it('converts lists, code blocks, quotes and tables', () => {
    const h = markdownToHTML('- one\n- two\n  - nested\n\n1. first\n2. second\n\n```js\nlet a = 1 < 2;\n```\n\n> quoted\n\n| a | b |\n|---|--:|\n| 1 | 2 |');
    expect(h).toContain('<ul><li>one</li><li>two<ul><li>nested</li></ul></li></ul>');
    expect(h).toContain('<ol><li>first</li><li>second</li></ol>');
    expect(h).toContain('<pre><code class="language-js">let a = 1 &lt; 2;</code></pre>');
    expect(h).toContain('<blockquote><p>quoted</p></blockquote>');
    expect(h).toContain('<th>a</th><th style="text-align:right">b</th>');
    expect(h).toContain('<td>1</td><td style="text-align:right">2</td>');
  });
});
