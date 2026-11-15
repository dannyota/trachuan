// @vitest-environment happy-dom
import { mount } from '@vue/test-utils'
import { afterEach, expect, it, vi } from 'vitest'
import { computed, ref } from 'vue'
import HomeConnect from '../app/components/HomeConnect.vue'
import Connect from '../app/pages/connect.vue'

afterEach(() => vi.unstubAllGlobals())

it('renders command blocks with their original text and unbroken tokens', () => {
  vi.stubGlobal('useState', () => ref('en'))
  vi.stubGlobal('computed', computed)
  vi.stubGlobal('useRuntimeConfig', () => ({ public: { siteOrigin: 'https://example.invalid' } }))
  const endpoint = 'https://example.invalid/mcp'
  const command = `claude mcp add --transport http trachuan ${endpoint}`
  const cursor = JSON.stringify({ mcpServers: { trachuan: { url: endpoint } } }, null, 2)
  const options = { global: { stubs: { CopyButton: true, NuxtLink: true } } }
  const home = mount(HomeConnect, options)
  const connect = mount(Connect, options)
  const blocks = [...home.findAll('.home-connect-card pre'), ...connect.findAll('.page-code pre')]
  const values = [command, endpoint, command, cursor]
  expect(blocks).toHaveLength(values.length)
  for (const [index, block] of blocks.entries()) {
    expect(block.element.textContent).toBe(values[index])
    expect(block.findAll('span').map(span => span.text())).toEqual(values[index]!.match(/\S+/g))
  }
  home.unmount()
  connect.unmount()
})
