import { execFileSync } from 'node:child_process'
import { expect, it } from 'vitest'

type Page = { status: number, headers: Record<string, string>, body: string }

function pages(available: boolean): Page[] {
  const script = `
    import { Server } from 'node:http';
    Server.prototype.listen = function () { return this };
    process.env.NITRO_SHUTDOWN_DISABLED = '1';
    globalThis.fetch = async request => {
      const path = new URL(request.url).pathname;
      const node = {code:'1',name:'First',description:'',provisions:1,documents:1,children:[]};
      const body = path.endsWith('/status') ? {release:'20990101_1',features:{public_summaries:${available}}}
        : path.endsWith('/codes') ? {release:'20990101_1',codes:['1']}
        : path.endsWith('/taxonomy/1') ? {node,path:[{code:'1',name:'First'}],documents:[{key:'vn:vbpl:1',number:'No. 1',title:'Document title',type:'law',status_class:'in_force',is_current:true}]}
        : {release:'20990101_1',projection:1,total:1,items:[{key:'vn:vbpl:1',index:true}]};
      return new Response(JSON.stringify(body), {headers:{'content-type':'application/json'}});
    };
    const { nitroApp: app } = await import('./tests/nitro-app.mjs');
    const pages = [];
    for (const path of ['/sitemap.xml','/sitemap-pages.xml','/sitemap-documents.xml','/vn/topics/1']) {
      const response = await app.localFetch(path);
      pages.push({status:response.status,headers:Object.fromEntries(response.headers),body:await response.text()});
    }
    console.log(JSON.stringify(pages));
  `
  return JSON.parse(execFileSync(process.execPath, ['--input-type=module', '-e', script], { encoding: 'utf8' }))
}

function documentLinks(body: string): string[] {
  return [...body.matchAll(/href="(\/vn\/documents\/[^"]*)"/g)].map(match => match[1]!)
}

it('keeps the fixed sitemap, hides the sitemap children and links topic documents to the reader when summaries are off', () => {
  const [index, fixed, documents, topic] = pages(false)
  expect(index!.status).toBe(200)
  expect(index!.body).toContain('<urlset')
  expect(index!.body).not.toContain('sitemap-documents.xml')
  for (const child of [fixed!, documents!]) {
    expect(child.status).toBe(404)
    expect(child.headers['cache-control']).toBe('no-cache')
  }
  expect(topic!.status).toBe(200)
  expect(documentLinks(topic!.body)).toEqual(['/vn/documents/vn:vbpl:1'])
})

it('links topic documents to their summaries when summaries are on', () => {
  const [index, , , topic] = pages(true)
  expect(index!.body).toContain('<sitemapindex')
  expect(documentLinks(topic!.body)).toEqual(['/vn/documents/vn:vbpl:1/summary'])
})
