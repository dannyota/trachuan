import tailwindcss from '@tailwindcss/vite'
import { APP_CSP } from './app/utils/csp'

const cache = { maxAge: 600, swr: false, varies: ['x-trachuan-language'] }

export default defineNuxtConfig({
  compatibilityDate: '2026-11-11',
  modules: ['@pinia/nuxt', '@nuxt/eslint'],
  css: ['~/assets/main.css'],
  features: { inlineStyles: false },
  experimental: { payloadExtraction: false, entryImportMap: false },
  vite: { plugins: [tailwindcss()] },
  runtimeConfig: { trustedProxies: '' },
  routeRules: {
    '/**': { headers: { 'Content-Security-Policy': APP_CSP } },
    '/': { cache },
    '/connect': { cache },
    '/sources': { cache },
    '/privacy': { cache },
    '/terms': { cache },
    '/llms.txt': { cache },
    '/login': { headers: { 'Cache-Control': 'no-store' } },
    '/account': { headers: { 'Cache-Control': 'no-store' } },
    '/vn/**': { headers: { 'Cache-Control': 'no-store' } },
  },
  nitro: { devProxy: { '/api/': { target: 'http://127.0.0.1:8080/api/', changeOrigin: true } } },
})
