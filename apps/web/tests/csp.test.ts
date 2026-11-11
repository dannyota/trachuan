import { describe, expect, it } from 'vitest'
import { externalizeNuxtBootstrap } from '../server/utils/cspExternalize'
import { APP_CSP } from '../app/utils/csp'

describe('content security policy', () => {
  it('restricts executable scripts, fonts and images to this origin', () => {
    expect(APP_CSP).toContain("script-src 'self'")
    expect(APP_CSP).toContain("style-src 'self'")
    expect(APP_CSP).toContain("img-src 'self'")
    expect(APP_CSP).toContain("font-src 'self'")
    expect(APP_CSP).not.toContain('unsafe-inline')
    expect(APP_CSP).not.toContain('unsafe-eval')
    expect(APP_CSP).not.toContain('https:')
  })
  it('moves the Nuxt payload to inert metadata and an external script', () => {
    const html = '<script type="application/json" data-nuxt-data="nuxt-app" data-ssr="true" id="__NUXT_DATA__">[1]</script><script>window.__NUXT__={};window.__NUXT__.config={}</script>'
    const result = externalizeNuxtBootstrap(html)
    expect(result).not.toContain('<script>')
    expect(result).toContain('<script src="/csp-bootstrap.js"></script>')
    expect(result).toContain('__NUXT_CSP_DATA__')
  })
})

describe('built public responses', () => {
  it('contain no inline scripts except JSON data blocks', async () => {
    const { execFileSync } = await import('node:child_process')
    const { Window } = await import('happy-dom')
    const script = `
      import { Server } from 'node:http';
      Server.prototype.listen = function () { return this };
      process.env.NITRO_SHUTDOWN_DISABLED = '1';
      await import('./.output/server/chunks/nitro/nitro.mjs');
      const routes = ['/', '/connect', '/sources', '/privacy', '/terms', '/llms.txt', '/login'];
      const pages = [];
      for (const route of routes) {
        const response = await globalThis.$fetch.raw(route);
        pages.push({ route, status: response.status, html: response._data });
      }
      console.log(JSON.stringify(pages));
    `
    const output = execFileSync(process.execPath, ['--input-type=module', '-e', script], { encoding: 'utf8' })
    const pages: Array<{ route: string, status: number, html: string }> = JSON.parse(output)
    for (const page of pages) {
      expect(page.status, page.route).toBe(200)
      const window = new Window()
      window.document.write(page.html)
      for (const script of window.document.querySelectorAll('script')) {
        if (!script.hasAttribute('src') || script.textContent.trim()) {
          expect(script.getAttribute('type'), page.route).toBe('application/json')
        }
      }
      await window.happyDOM.close()
    }
  })
})
