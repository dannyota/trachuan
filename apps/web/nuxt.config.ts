import tailwindcss from '@tailwindcss/vite'
import { APP_CSP } from './app/utils/csp'

const accountHeaders = { 'Cache-Control': 'no-store', 'X-Robots-Tag': 'noindex' }
const cache = { maxAge: 600, swr: false, varies: ['x-trachuan-language'] }
const sitemapCache = { ...cache, maxAge: 300 }
const summaryCache = { ...sitemapCache, base: 'summary', varies: ['x-trachuan-language', 'x-trachuan-summary-date'] }

export default defineNuxtConfig({
  compatibilityDate: '2026-11-11',
  modules: ['@pinia/nuxt', '@nuxt/eslint'],
  app: { head: { title: 'Tra Chuẩn' } },
  css: ['~/assets/main.css'],
  features: { inlineStyles: false },
  experimental: { payloadExtraction: false, entryImportMap: false },
  vite: { plugins: [tailwindcss()] },
  runtimeConfig: { trustedProxies: '', apiOrigin: 'http://127.0.0.1:8080', public: { siteOrigin: 'https://trachuan.vn' } },
  routeRules: {
    '/**': { headers: { 'Content-Security-Policy': APP_CSP } },
    '/api/v1/frameworks': { cache: false, headers: { 'Cache-Control': 'no-store' } },
    '/api/v1/frameworks/**': { cache: false, headers: { 'Cache-Control': 'no-store' } },
    '/': { cache },
    '/connect': { cache },
    '/sources': { cache },
    '/privacy': { cache },
    '/terms': { cache },
    '/llms.txt': { cache },
    '/robots.txt': { cache },
    '/sitemap.xml': { cache: sitemapCache },
    '/sitemap-pages.xml': { cache: sitemapCache },
    '/sitemap-documents.xml': { cache: sitemapCache },
    '/frameworks': { cache: false, headers: accountHeaders },
    '/frameworks/**': { cache: false, headers: accountHeaders },
    '/login': { headers: accountHeaders },
    '/login/factor': { headers: accountHeaders },
    '/signup': { headers: accountHeaders },
    '/reset': { headers: accountHeaders },
    '/link': { headers: accountHeaders },
    '/account': { headers: accountHeaders },
    '/vn/topics': { cache, headers: { 'Cache-Control': 'public, max-age=600' } },
    '/vn/topics/**': { cache, headers: { 'Cache-Control': 'public, max-age=600' } },
    '/vn/documents/*/summary': { cache: summaryCache, headers: { 'Cache-Control': 'public, max-age=300' } },
    '/vn/**': { headers: { 'Cache-Control': 'no-store' } },
  },
  nitro: { devProxy: { '/api/': { target: 'http://127.0.0.1:8080/api/', changeOrigin: true } } },
})
