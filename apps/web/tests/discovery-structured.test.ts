import { execFileSync } from 'node:child_process'
import { Window } from 'happy-dom'
import { expect, it } from 'vitest'
import { topicSeo } from '../app/utils/topic-seo'

it('cuts long topic names and descriptions at word boundaries', () => {
  for (const lang of ['vi', 'en'] as const) {
    const result = topicSeo({ name: 'Chủ đề pháp luật '.repeat(8), description: 'Mô tả chủ đề '.repeat(30) }, lang)
    expect(result.title.normalize('NFC').length).toBeLessThanOrEqual(60)
    expect(result.description.normalize('NFC').length).toBeLessThanOrEqual(160)
    expect(result.title.split(' · ')[0]).toMatch(/(?:Chủ|đề|pháp|luật)$/)
    expect(result.description).toMatch(/(?:Mô|tả|chủ|đề)$/)
    const fallback = topicSeo({ name: 'Chủ đề pháp luật '.repeat(30), documents: 1200, provisions: 3400 }, lang).description
    expect(fallback.length).toBeLessThanOrEqual(160)
    expect(fallback).toContain(lang === 'vi' ? '1.200 văn bản, 3.400 quy định.' : '1,200 documents, 3,400 provisions.')
    const unbroken = topicSeo({ code: '12.3', name: 'x'.repeat(200), description: 'y'.repeat(200), documents: 1, provisions: 2 }, lang)
    expect(unbroken.title).toBe(`12.3 · ${lang === 'vi' ? 'Chủ đề' : 'Topics'} · Tra Chuẩn`)
    expect(unbroken.description).not.toMatch(/xxx|yyy/)
    expect(unbroken.description).toContain('“12.3”')
    expect(unbroken.description.length).toBeLessThanOrEqual(160)
  }
})

it('renders safe structured data and localized source facts', () => {
  const script = `
    import { Server } from 'node:http';
    Server.prototype.listen = function () { return this };
    process.env.NITRO_SHUTDOWN_DISABLED = '1';
    const newest = process.argv[1] !== 'missing';
    globalThis.fetch = async request => {
      const path = new URL(request.url).pathname;
      const node = { code: '1', name: 'Chủ đề', documents: 1200, provisions: 3400 };
      const data = path.endsWith('/taxonomy/1') ? { node, path: [node], documents: [{ title: '</script><script>alert(1)</script>', number: '1', type: 'Luật' }] } : { release: '20990101_1', documents: 1200, provisions: 3400, nodes: 5, ...(newest ? { newest_source_date: '2099-01-02' } : {}) };
      return new Response(JSON.stringify(data), { headers: { 'content-type': 'application/json' } });
    };
    const app = (await import('./.output/server/chunks/nitro/nitro.mjs')).g();
    const pages = [];
    for (const lang of ['vi', 'en']) for (const path of ['/', '/vn/topics/1', '/sources']) {
      const response = await app.localFetch(path + '?lang=' + lang, { headers: { cookie: 'lang=' + lang } });
      pages.push(await response.text());
    }
    pages.push(await (await app.localFetch('/llms.txt')).text());
    console.log(JSON.stringify(pages));
  `
  const pages: string[] = JSON.parse(execFileSync(process.execPath, ['--input-type=module', '-e', script], { encoding: 'utf8' }))
  const missing: string[] = JSON.parse(execFileSync(process.execPath, ['--input-type=module', '-e', script, 'missing'], { encoding: 'utf8' }))
  for (const [index, html] of pages.slice(0, 6).entries()) {
    const document = new Window().document
    document.write(html)
    const blocks = document.querySelectorAll('script[type="application/ld+json"]')
    expect(blocks).toHaveLength(1)
    const graph = JSON.parse(blocks[0]!.textContent)['@graph']
    expect(graph.map((node: { '@type': string }) => node['@type'])).toEqual(index % 3 === 0 ? ['WebSite', 'Organization', 'WebApplication', 'Person'] : index % 3 === 1 ? ['BreadcrumbList', 'ItemList'] : ['Dataset'])
    expect(JSON.stringify(graph)).not.toMatch(/legislationLegalForce|"status"/)
    if (index % 3 === 1) {
      expect(graph[1].itemListElement[0].name).toBe('</script><script>alert(1)</script>')
      expect(html).not.toContain('</script><script>alert(1)</script>')
    }
    if (index % 3 === 2) {
      expect(graph[0].dateModified).toBe('2099-01-01')
      expect(graph[0].description.length).toBeGreaterThanOrEqual(50)
      expect(graph[0].description.length).toBeLessThanOrEqual(5000)
      expect(graph[0].license).toBeUndefined()
    }
  }
  expect(pages[2]).toContain('Bản phát hành 01/01/2099 (bản 1) có 1.200 văn bản và 3.400 quy định; nguồn mới nhất ngày 02/01/2099.')
  expect(pages[5]).toContain('Release 01 Jan 2099 (build 1) has 1,200 documents and 3,400 provisions; the newest source is dated 02 Jan 2099.')
  expect(missing[2]).toContain('Bản phát hành 01/01/2099 (bản 1) có 1.200 văn bản và 3.400 quy định.')
  expect(missing[5]).toContain('Release 01 Jan 2099 (build 1) has 1,200 documents and 3,400 provisions.')
  expect(pages[6]).toMatch(/^# Tra Chuẩn\n\n> Tra Chuẩn/)
  expect(pages[6]).toContain('Search, then scan documents and hits. Read chosen handles together, using a path anchor to read an article or chapter. Check status, warnings and incoming repeals, replaces, suspends and amends_supplements pointers. Read incoming implements documents for detailed rules. Follow next until the last page.')
  expect(pages[6]).toContain('A snippet is not the provision; […] marks omitted text. Pointer direction is as recorded by the source. Do not replace a pointer outside the release with a number lookup. Cite only complete returned text and cite the returned evidence in answers.')
  for (const path of ['/mcp', '/connect', '/sources', '/vn/topics', '/privacy', '/terms']) expect(pages[6]).toContain(`https://trachuan.vn${path}`)
})
