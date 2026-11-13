import { globSync, readFileSync } from 'node:fs'
import { expect, it } from 'vitest'

const families = {
  BeVietnam: 'be-vietnam-pro-var',
  NotoSerif: 'noto-serif-var',
  RobotoMono: 'roboto-mono-var',
}

it('declares one same-origin face per family with the available weights', () => {
  for (const [path, names] of [
    ['app/assets/fonts.css', Object.keys(families)],
    ['public/static-page.css', ['BeVietnam', 'NotoSerif']],
  ] as const) {
    const css = readFileSync(path, 'utf8')
    const faces = [...css.matchAll(/@font-face\s*\{([^}]*)\}/g)].map(match => match[1]!)
    expect(faces).toHaveLength(names.length)
    for (const [index, name] of names.entries()) {
      const face = faces[index]!
      expect(face).toMatch(new RegExp(`font-family:\\s*${name}\\s*;`))
      expect(face).toContain(`src: url('/fonts/${families[name as keyof typeof families]}.woff2') format('woff2');`)
      expect(face).toMatch(/font-weight:\s*400 700\s*;/)
      expect(face).toMatch(/font-display:\s*swap\s*;/)
      expect(face).not.toMatch(/unicode-range\s*:/)
    }
  }
})

it('keeps app CSS weights within every font family range', () => {
  const paths = globSync('app/**/*.css').filter(path => path !== 'app/assets/fonts.css')
  expect(paths.length).toBeGreaterThan(0)
  for (const path of paths) {
    const css = readFileSync(path, 'utf8')
    for (const match of css.matchAll(/font-weight\s*:\s*([^;}]+)/g)) {
      const weight = Number(match[1]!.trim())
      expect(weight, `${path}: ${match[0]}`).toBeGreaterThanOrEqual(400)
      expect(weight, `${path}: ${match[0]}`).toBeLessThanOrEqual(700)
    }
  }
})
