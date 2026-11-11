import { Buffer } from 'node:buffer';

const CONFIG_PATTERN
  = new RegExp([
    '<script>window\\.__NUXT__=\\{\\};',
    'window\\.__NUXT__\\.config=([\\s\\S]*?)<\\/script>',
  ].join(''), 'g');
const PAYLOAD_PATTERN
  = new RegExp([
    '<script(?=[^>]*\\btype="application\\/json")',
    '(?=[^>]*\\bid="__NUXT_DATA__")',
    '([^>]*)>([\\s\\S]*?)<\\/script>',
  ].join(''), 'g');

const encoded = (value: string): string =>
  Buffer.from(value, 'utf8').toString('base64');

export function externalizeNuxtBootstrap(
  html: string,
  clientConfig?: unknown,
): string {
  const configs = [...html.matchAll(CONFIG_PATTERN)];
  const payloads = [...html.matchAll(PAYLOAD_PATTERN)];
  if (configs.length === 0 && payloads.length === 0) return html;
  if (configs.length !== 1 || payloads.length !== 1) {
    throw new Error('Nuxt bootstrap must contain config and payload together.');
  }

  const config = configs[0]?.[1];
  const payloadAttributes = payloads[0]?.[1];
  const payload = payloads[0]?.[2];
  if (
    config === undefined
    || payloadAttributes === undefined
    || payload === undefined
  ) {
    throw new Error('Nuxt bootstrap capture failed.');
  }
  const nuxtData = /\bdata-nuxt-data="([^"]+)"/.exec(payloadAttributes)?.[1];
  const serverRendered = /\bdata-ssr="([^"]+)"/.exec(payloadAttributes)?.[1];
  if (nuxtData === undefined || serverRendered === undefined) {
    throw new Error('Nuxt payload attributes are incomplete.');
  }

  return html
    .replace(
      configs[0]![0],
      '<meta id="__NUXT_CSP_CONFIG__" content="'
      + `${encoded(clientConfig === undefined
        ? config
        : JSON.stringify(clientConfig))}">`,
    )
    .replace(
      payloads[0]![0],
      '<meta id="__NUXT_CSP_DATA__"'
      + ` data-nuxt-data="${nuxtData}" data-ssr="${serverRendered}"`
      + ` content="${encoded(payload)}">`
      + '<script src="/csp-bootstrap.js"></script>',
    );
}
