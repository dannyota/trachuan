import { expect, it } from 'vitest'
import { publicHead } from '../i18n/public-head'

it('uses the discovery title and description for the home page', () => {
  expect(publicHead('vi', '/')).toEqual({
    title: 'Tra Chuẩn · Tra cứu nguyên văn pháp luật Việt Nam',
    description: 'Hỏi bằng tiếng Việt, đọc nguyên văn điều, khoản trong văn bản pháp luật Việt Nam, kèm số hiệu và trạng thái hiệu lực. Dùng trên web hoặc qua MCP.',
  })
})
