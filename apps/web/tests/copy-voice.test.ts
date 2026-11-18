import { readFileSync, readdirSync } from 'node:fs'
import { join } from 'node:path'
import { expect, it } from 'vitest'
import { vi as shared } from '../i18n/vi'
import { homeMessages } from '../i18n/home'
import { pagesMessages } from '../i18n/pages'
import { frameworkMessages } from '../i18n/frameworks'
import { accountMessages } from '../i18n/account'
import { authMessages } from '../i18n/auth'
import { readerMessages } from '../i18n/reader'
import { searchMessages } from '../i18n/search'
import { shellMessages } from '../i18n/shell'
import { summaryMessages } from '../i18n/summary'
import { legalCopy } from '../i18n/legal'
import { publicHead } from '../i18n/public-head'

function collect(value: unknown, out: string[] = []): string[] {
  if (typeof value === 'string') out.push(value)
  else if (typeof value === 'function') out.push((value as (...args: string[]) => string)('1', '2', '3'))
  else if (Array.isArray(value)) value.forEach(item => collect(item, out))
  else if (value && typeof value === 'object') Object.values(value).forEach(item => collect(item, out))
  return out
}

const promotional = /(mạnh mẽ|toàn diện|thông minh|dễ dàng|nhanh chóng|tối ưu|hàng đầu|tuyệt vời|liền mạch)/i
const wordRules: Array<[string, (text: string) => boolean, string]> = [
  ['promotional word', text => promotional.test(text), 'Tìm kiếm thông minh.'],
  ['không chỉ ... mà còn', text => /không chỉ(?! ra)[^.]*mà còn/i.test(text), 'Không chỉ tìm mà còn đọc.'],
  ['giúp bạn opener', text => /(^|[.!?'`>]\s*)giúp bạn/i.test(text), "'Giúp bạn tra cứu'"],
  ['translated phrasing', text => /(^|[\s('`>])(thực hiện|tiến hành|nhằm|đối với|cho phép bạn|bằng một lần bấm)(?=[\s,.;:)'`<]|$)/i.test(text), "'Tiến hành tra cứu'"],
  ['giúp bạn opener after a backtick', text => /(^|[.!?'`>]\s*)giúp bạn/i.test(text), '`Giúp bạn tra cứu`'],
  ['giúp bạn opener after >', text => /(^|[.!?'`>]\s*)giúp bạn/i.test(text), '<p>Giúp bạn tra cứu</p>'],
  ['translated phrasing after a backtick', text => /(^|[\s('`>])(thực hiện|tiến hành|nhằm|đối với|cho phép bạn|bằng một lần bấm)(?=[\s,.;:)'`<]|$)/i.test(text), '`Tiến hành tra cứu`'],
  ['translated phrasing after >', text => /(^|[\s('`>])(thực hiện|tiến hành|nhằm|đối với|cho phép bạn|bằng một lần bấm)(?=[\s,.;:)'`<]|$)/i.test(text), '<p>Tiến hành tra cứu</p>'],
  ['stiff error phrasing', text => /(vui lòng|đã xảy ra lỗi|không thể)/i.test(text), 'Vui lòng thử lại.'],
]
const punctuationRules: Array<[string, (text: string) => boolean, string]> = [
  ['exclamation mark', text => text.includes('!'), 'Xin chào!'],
  ['em dash', text => text.includes('\u2014'), 'Một \u2014 hai'],
]
const home = Object.fromEntries(Object.entries(homeMessages('vi')).filter(([key]) => !key.startsWith('maker')))

const wordStrings = [
  ...collect(shared),
  ...collect(home),
  ...collect(pagesMessages('vi')),
  ...collect(frameworkMessages('vi')),
  ...collect(accountMessages('vi')),
  ...collect(authMessages('vi')),
  ...collect(readerMessages('vi')),
  ...collect(searchMessages('vi')),
  ...collect(shellMessages('vi')),
  ...collect(summaryMessages('vi')),
  ...['/', '/vn/topics', '/connect', '/sources', '/privacy', '/terms', '/login', '/login/factor', '/signup', '/reset', '/link'].flatMap(path => collect(publicHead('vi', path))),
]
const allStrings = [...wordStrings, ...collect(homeMessages('vi')), ...collect(legalCopy('vi'))]

function files(dir: string): string[] {
  return readdirSync(dir, { withFileTypes: true }).flatMap(entry => entry.isDirectory() ? files(join(dir, entry.name)) : [join(dir, entry.name)])
}
const sources = [...files('app/content'), ...files('app/components'), ...files('app/pages'), 'app/utils/topic-seo.ts', 'app/utils/structured-data.ts', 'server/utils/not-found.ts']
  .filter(path => /\.(ts|vue)$/.test(path))

it.each([...wordRules, ...punctuationRules])('rule fires on a sample: %s', (_name, broken, sample) => {
  expect(broken(sample)).toBe(true)
})

it.each(wordRules)('Vietnamese dictionaries avoid: %s', (_name, broken) => {
  expect(wordStrings.filter(broken)).toEqual([])
})

it.each(punctuationRules)('Vietnamese dictionaries and legal text avoid: %s', (_name, broken) => {
  expect(allStrings.filter(broken)).toEqual([])
})

it.each(wordRules)('Vietnamese source files avoid: %s', (_name, broken) => {
  const hits = sources.flatMap(path => readFileSync(path, 'utf8').split('\n').filter(broken).map(line => `${path}: ${line.trim().slice(0, 120)}`))
  expect(hits).toEqual([])
})
