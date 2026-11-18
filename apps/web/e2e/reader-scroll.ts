import { expect, type Locator } from '@playwright/test'

export async function scrollTo(target: Locator, top: number) {
  await expect.poll(() => target.evaluate(async (element, top) => {
    window.scrollTo(0, window.scrollY + element.getBoundingClientRect().top - top)
    await new Promise<void>(resolve => requestAnimationFrame(() => requestAnimationFrame(() => resolve())))
    return Math.abs(element.getBoundingClientRect().top - top) <= 2
  }, top)).toBe(true)
}
