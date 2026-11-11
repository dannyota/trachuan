(() => {
  'use strict';

  const decode = (element) => {
    const bytes = Uint8Array.from(
      atob(element.getAttribute('content') ?? ''),
      character => character.charCodeAt(0),
    );
    return new TextDecoder().decode(bytes);
  };

  const config = document.getElementById('__NUXT_CSP_CONFIG__');
  const payload = document.getElementById('__NUXT_CSP_DATA__');
  if (!(config instanceof HTMLMetaElement)) {
    throw new Error('Missing externalized Nuxt config.');
  }
  if (!(payload instanceof HTMLMetaElement)) {
    throw new Error('Missing externalized Nuxt payload.');
  }

  window.__NUXT__ = {};
  window.__NUXT__.config = JSON.parse(decode(config));
  const data = document.createElement('script');
  data.id = '__NUXT_DATA__';
  data.type = 'application/json';
  data.dataset.nuxtData = payload.dataset.nuxtData ?? '';
  data.dataset.ssr = payload.dataset.ssr ?? '';
  data.textContent = decode(payload);
  payload.replaceWith(data);
  config.remove();
})();
