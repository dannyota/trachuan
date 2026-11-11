import { describe, expect, it } from 'vitest'
import { createAdmission, clientAddress } from '../server/utils/public-admission'

describe('public admission', () => {
  it('rejects the 121st request and refills after one minute', () => {
    let now = 0
    const admission = createAdmission(() => now)
    for (let i = 0; i < 120; i++) expect(admission.allow('192.0.2.1')).toBe(true)
    expect(admission.allow('192.0.2.1')).toBe(false)
    now = 60_000
    expect(admission.allow('192.0.2.1')).toBe(true)
  })
  it('rejects a fifth concurrent render and releases capacity', () => {
    const admission = createAdmission(() => 0)
    const releases = Array.from({ length: 4 }, () => admission.render())
    expect(admission.render()).toBeNull()
    releases[0]?.()
    expect(admission.render()).not.toBeNull()
  })
  it('trusts only configured proxies and rejects malformed chains', () => {
    const ranges = ['10.0.0.0/8', '2001:db8::/32']
    expect(clientAddress('10.0.0.2', '192.0.2.5, 10.0.0.1', ranges)).toBe('192.0.2.5')
    expect(clientAddress('10.0.0.2', 'bad, 10.0.0.1', ranges)).toBe('10.0.0.2')
    expect(clientAddress('192.0.2.2', '192.0.2.5', ranges)).toBe('192.0.2.2')
    expect(clientAddress('2001:db8::1', '2001:db9::1', ranges)).toBe('2001:db9::1')
  })
})

it('does not acquire a render slot for a cached response', async () => {
  const { execFileSync } = await import('node:child_process')
  const script = `
    import { Server } from 'node:http';
    Server.prototype.listen = function () { return this };
    process.env.NITRO_SHUTDOWN_DISABLED = '1';
    const nitro = await import('./.output/server/chunks/nitro/nitro.mjs');
    const app = nitro.g();
    const warm = await app.localFetch('/');
    if (warm.status !== 200) throw new Error('warm failed');
    let started = 0;
    let allStarted;
    const ready = new Promise(resolve => { allStarted = resolve });
    let release;
    const blocked = new Promise(resolve => { release = resolve });
    app.hooks.hook('render:before', async () => {
      started++;
      if (started === 4) allStarted();
      await blocked;
    });
    const pending = ['/connect', '/sources', '/privacy', '/terms'].map(path => app.localFetch(path));
    await ready;
    const hit = await app.localFetch('/');
    console.log(JSON.stringify({ status: hit.status, renders: started }));
    release();
    await Promise.all(pending);
  `
  const result = execFileSync(process.execPath, ['--input-type=module', '-e', script], { encoding: 'utf8', timeout: 5000 })
  expect(JSON.parse(result)).toEqual({ status: 200, renders: 4 })
})
