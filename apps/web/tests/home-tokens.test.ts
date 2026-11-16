import { readFileSync } from 'node:fs'
import { expect, it } from 'vitest'

const expected = [
  { 'page': '#FFFFFF', 'card': '#FFFFFF', 'mist': '#F1F4FA', 'sky': '#E3EAF8', 'ink': '#0F1524', 'muted': '#4D556A', 'line': '#E1E5EE', 'control': '#6F7A9B', 'primary-fill': '#1E3A8A', 'primary-hover': '#172F70', 'primary': '#1E3A8A', 'on-primary': '#FFFFFF', 'primary-hint': '#D3DCF6', 'hero-field': '#FFFFFF', 'hero-field-ink': '#0F1524', 'hero-placeholder': '#4D556A', 'hero-pill': '#FFFFFF', 'hero-pill-ink': '#1E3A8A', 'footer': '#0F1524', 'footer-ink': '#FFFFFF', 'footer-muted': '#C3CADB', 'ring': '#0F1524', 'ring-inverse': '#FFFFFF', 'footer-ring': '#FFFFFF', 'seal': '#B42318', 'seal-footer': '#E5604F', 'danger': '#B42318', 'danger-bg': '#FEF3F2', 'danger-line': '#F3C7C1', 'highlight': '#FFF1A8', 'highlight-ink': '#0F1524', 'note-warn': '#FDF6E9', 'ink-in-force': '#0B6E63', 'stamp-in-force-bg': '#E3F4F0', 'ink-partial': '#8A4B08', 'stamp-partial-bg': '#FDF1DC', 'ink-not-yet': '#1D4ED8', 'stamp-not-yet-bg': '#E4ECFE', 'ink-expired': '#4A5160', 'stamp-expired-bg': '#ECEEF1', 'stamp-neutral-border': '#6B7280' },
  { 'page': '#0C1222', 'card': '#161E33', 'mist': '#131A2E', 'sky': '#1B2A52', 'ink': '#EEF1F8', 'muted': '#A3ABC2', 'line': '#283048', 'control': '#6F7DA3', 'primary-fill': '#3451B2', 'primary-hover': '#2C469C', 'primary': '#9DB4F5', 'on-primary': '#FFFFFF', 'primary-hint': '#D3DCF6', 'hero-field': '#EEF1F8', 'hero-field-ink': '#0F1524', 'hero-placeholder': '#4D556A', 'hero-pill': '#FFFFFF', 'hero-pill-ink': '#1E3A8A', 'footer': '#131A2E', 'footer-ink': '#EEF1F8', 'footer-muted': '#A3ABC2', 'ring': '#9DB4F5', 'ring-inverse': '#FFFFFF', 'footer-ring': '#9DB4F5', 'seal': '#E5604F', 'seal-footer': '#E5604F', 'danger': '#E5604F', 'danger-bg': '#2A1412', 'danger-line': '#5A2620', 'highlight': '#4A3F12', 'highlight-ink': '#F5ECC0', 'note-warn': '#2B2110', 'ink-in-force': '#5ED0BD', 'stamp-in-force-bg': '#0F2C28', 'ink-partial': '#F0B565', 'stamp-partial-bg': '#33230B', 'ink-not-yet': '#9DB8FF', 'stamp-not-yet-bg': '#16224A', 'ink-expired': '#B3B9C4', 'stamp-expired-bg': '#23272E', 'stamp-neutral-border': '#6B7280' },
]

const pairs: [string, string, number][] = [
  ...['page', 'card', 'mist', 'sky', 'note-warn'].flatMap(ground => [['ink', ground, 4.5], ['muted', ground, 4.5]] as [string, string, number][]),
  ...['page', 'card', 'mist', 'sky'].flatMap(ground => [['primary', ground, 4.5], ['control', ground, 3], ['ring', ground, 3]] as [string, string, number][]),
  ...['in-force', 'partial', 'not-yet', 'expired'].flatMap(stamp => [[`ink-${stamp}`, `stamp-${stamp}-bg`, 4.5], ...['page', 'card', 'mist'].map(ground => [`ink-${stamp}`, ground, 3] as [string, string, number])] as [string, string, number][]),
  ...['page', 'card', 'mist'].map(ground => ['stamp-neutral-border', ground, 3] as [string, string, number]),
  ['highlight-ink', 'highlight', 4.5], ['on-primary', 'primary-fill', 4.5], ['on-primary', 'primary-hover', 4.5], ['primary-hint', 'primary-fill', 4.5], ['footer-ink', 'footer', 4.5], ['footer-muted', 'footer', 4.5], ['danger', 'page', 4.5], ['danger', 'card', 4.5], ['danger', 'danger-bg', 4.5], ['ring-inverse', 'primary-fill', 3], ['footer-ring', 'footer', 3], ['hero-field', 'primary-fill', 3], ['hero-field-ink', 'hero-field', 4.5], ['hero-placeholder', 'hero-field', 4.5], ['hero-pill', 'primary-fill', 3], ['hero-pill-ink', 'hero-pill', 4.5], ['seal', 'page', 3], ['seal', 'mist', 3], ['seal-footer', 'footer', 3],
]

function luminance(hex: string) {
  const channels = hex.slice(1).match(/../g)!.map(value => parseInt(value, 16) / 255).map(value => value <= 0.04045 ? value / 12.92 : ((value + 0.055) / 1.055) ** 2.4)
  return channels[0]! * 0.2126 + channels[1]! * 0.7152 + channels[2]! * 0.0722
}

it('pins global tokens and their contrast in both themes', () => {
  const css = readFileSync('app/assets/tokens.css', 'utf8')
  const blocks = [...css.matchAll(/(?:@theme static|:root)\s*\{([^}]+)\}/g)]
  expect(blocks).toHaveLength(2)
  for (const [index, values] of expected.entries()) {
    const actual = Object.fromEntries([...blocks[index]![1]!.matchAll(/--color-([\w-]+):\s*(#[0-9A-F]{6})/g)].map(match => [match[1], match[2]]))
    expect(actual).toEqual(values)
    const tokens: Record<string, string> = values
    for (const [foreground, ground, minimum] of pairs) {
      const a = luminance(tokens[foreground]!)
      const b = luminance(tokens[ground]!)
      expect((Math.max(a, b) + 0.05) / (Math.min(a, b) + 0.05), `${index}: ${foreground} on ${ground}`).toBeGreaterThanOrEqual(minimum)
    }
  }
})


it('keeps static error page colours equal to global tokens', () => {
  const css = readFileSync('public/static-page.css', 'utf8')
  const blocks = [...css.matchAll(/:root\s*\{([^}]+)\}/g)]
  expect(blocks).toHaveLength(2)
  for (const [index, values] of expected.entries()) {
    const actual = Object.fromEntries([...blocks[index]![1]!.matchAll(/--color-([\w-]+):\s*(#[0-9A-F]{6})/g)].map(match => [match[1], match[2]]))
    expect(actual).toEqual(values)
  }
})
