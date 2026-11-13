import { readFile, writeFile } from 'node:fs/promises'
import { fileURLToPath } from 'node:url'
import { chromium } from '@playwright/test'

const publicDir = fileURLToPath(new URL('../public/', import.meta.url))
const font = (await readFile(`${publicDir}fonts/noto-serif-var.woff2`)).toString('base64')
const sans = (await readFile(`${publicDir}fonts/be-vietnam-pro-var.woff2`)).toString('base64')
const svg = '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32"><circle cx="16" cy="16" r="14.5" fill="#B42318"/><text x="16" y="21.5" text-anchor="middle" font-family="Noto Serif, Georgia, serif" font-size="15" font-weight="700" fill="#FFFFFF">TC</text></svg>'
await writeFile(`${publicDir}icon.svg`, svg)
const browser = await chromium.launch({ headless: true })
const page = await browser.newPage({ deviceScaleFactor: 1 })
const fonts = `@font-face{font-family:Noto;src:url(data:font/woff2;base64,${font});font-weight:100 900} @font-face{font-family:Be;src:url(data:font/woff2;base64,${sans});font-weight:100 900}`
async function render(width, height, content) {
  await page.setViewportSize({ width, height })
  await page.setContent(`<style>${fonts}html,body{margin:0;width:100%;height:100%;overflow:hidden}svg{display:block}</style>${content}`)
  await page.evaluate(() => document.fonts.ready)
  return page.screenshot({ omitBackground: true })
}
function small(size) {
  return `<svg width="${size}" height="${size}" viewBox="0 0 ${size} ${size}" xmlns="http://www.w3.org/2000/svg"><circle cx="${size / 2}" cy="${size / 2}" r="${size / 2 - .5}" fill="#B42318"/><text x="${size / 2}" y="${size === 16 ? 11.6 : 21.5}" text-anchor="middle" font-family="Noto" font-size="${size === 16 ? 10 : 15}" font-weight="700" fill="white">${size === 16 ? 'T' : 'TC'}</text></svg>`
}
function large(size) {
  return `<svg width="${size}" height="${size}" viewBox="0 0 120 120" xmlns="http://www.w3.org/2000/svg"><rect width="120" height="120" fill="white"/><circle cx="60" cy="60" r="52" fill="none" stroke="#B42318" stroke-width="6"/><circle cx="60" cy="60" r="40" fill="none" stroke="#B42318" stroke-width="2"/><text x="60" y="74" text-anchor="middle" font-family="Noto" font-size="40" font-weight="700" fill="#B42318">TC</text></svg>`
}
const png16 = await render(16, 16, small(16))
const png32 = await render(32, 32, small(32))
const ico = Buffer.alloc(38)
ico.writeUInt16LE(1, 2); ico.writeUInt16LE(2, 4)
for (const [index, png] of [png16, png32].entries()) {
  const offset = 6 + index * 16
  ico[offset] = index === 0 ? 16 : 32; ico[offset + 1] = ico[offset]
  ico.writeUInt16LE(1, offset + 4); ico.writeUInt16LE(32, offset + 6)
  ico.writeUInt32LE(png.length, offset + 8); ico.writeUInt32LE(38 + (index ? png16.length : 0), offset + 12)
}
await writeFile(`${publicDir}favicon.ico`, Buffer.concat([ico, png16, png32]))
for (const [size, name] of [[180, 'apple-touch-icon.png'], [512, 'icon-512.png']]) await writeFile(`${publicDir}${name}`, await render(size, size, large(size)))
await writeFile(`${publicDir}og-image.png`, await render(1200, 630, `<div style="box-sizing:border-box;width:1200px;height:630px;background:white;color:#14171C;padding:110px;font-family:Be,sans-serif"><div style="display:flex;align-items:center;gap:28px">${large(120)}<strong style="font-size:54px">Tra Chuẩn</strong></div><h1 style="font-size:48px;line-height:1.2;max-width:900px">Tra cứu đúng điều, đúng khoản, kèm trạng thái hiệu lực.</h1><p style="color:#59606B;font-size:26px">trachuan.vn</p></div>`))
await browser.close()
