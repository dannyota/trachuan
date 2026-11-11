import { spawnSync } from 'node:child_process'
import { resolve } from 'node:path'
import { tmpdir } from 'node:os'
import { expect, it } from 'vitest'

it('finds the evaluation case regardless of the working directory', () => {
  const result = spawnSync(process.execPath, [resolve('scripts/make-sample.mjs')], {
    cwd: tmpdir(), env: {}, encoding: 'utf8',
  })
  expect(result.status).not.toBe(0)
  expect(result.stderr).toContain('Sample input is unavailable')
  expect(result.stderr).not.toContain('ENOENT')
})
