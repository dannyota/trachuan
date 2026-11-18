import type { Page } from '@playwright/test'

export async function hydrated(page: Page) {
  await page.waitForFunction(() => {
    const app = (document.getElementById('__nuxt') as unknown as { __vue_app__?: { config: { globalProperties: { $nuxt?: { isHydrating: boolean } } } } } | null)?.__vue_app__
    const nuxt = app?.config.globalProperties.$nuxt
    return nuxt !== undefined && !nuxt.isHydrating
  })
}
