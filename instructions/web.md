# Web

Read for `apps/web` (Nuxt 4, Vue 3, TypeScript).

- Stack as in aboutme.vn: Nuxt 4 SSR, Pinia, reka-ui, Tailwind, `openapi-fetch` client generated from `packages/schema/openapi.yaml`. CI fails on client drift.
- Routes are English with a country segment: `/vn/search`, `/vn/documents/{key}`, `/vn/topics/{code}`, `/connect`, `/sources`, `/account`. Interface text defaults to Vietnamese with optional English; legal text is never translated.
- Public pages render without a session and are cached with Nitro route rules. Signed-in pages call `/api/v1/*` with the session cookie and CSRF token.
- Self-host fonts; CSP allows no third-party host. Theme follows the device.
- Component files at most 400 lines; split by concern. Vitest for logic, Playwright for flows. Tests: `<subject>.test.ts`, `<feature>[-<scenario>].spec.ts`.
- Never put secrets or the Google client secret in web code or env exposed to the browser.
