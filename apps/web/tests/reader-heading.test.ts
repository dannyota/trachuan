import { expect, it } from 'vitest'
import { headingIndex } from '../app/utils/reader-units'

it('finds the last heading at the sticky edge by position, including duplicate labels', () => {
  let reads = 0
  const headings = Array.from({ length: 300 }, (_, index) => ({
    dataset: { spyAnchor: index === 150 ? 'dieu-2~11' : `dieu-${index + 1}` },
    textContent: index === 150 ? 'Điều 2' : `Điều ${index + 1}`,
    getBoundingClientRect: () => { reads++; return { top: index * 10 } },
  })) as unknown as HTMLElement[]
  expect(headingIndex([], 10)).toBe(-1)
  expect(headingIndex(headings, -1)).toBe(-1)
  expect(headingIndex(headings, 1500)).toBe(150)
  expect(headings[headingIndex(headings, 1500)]!.dataset.spyAnchor).toBe('dieu-2~11')
  expect(headingIndex(headings, 1509)).toBe(150)
  expect(headingIndex(headings, 5000)).toBe(299)
  expect(headingIndex([headings[1]!, headings[1]!], 10)).toBe(1)
  reads = 0
  expect(headingIndex(headings, 1500)).toBe(150)
  expect(reads).toBeLessThanOrEqual(9)
})
