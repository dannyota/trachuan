// @vitest-environment happy-dom
import { mount } from '@vue/test-utils'
import { h } from 'vue'
import { expect, it } from 'vitest'
import LegalText from '../app/components/LegalText.vue'
import { legalCopy } from '../i18n/legal'

it('links the support address without changing the legal text', () => {
  for (const language of ['vi', 'en'] as const) {
    const copy = legalCopy(language)
    for (const [, text] of [...copy.privacy, ...copy.terms]) {
      const wrapper = mount({ render: () => h('p', [h(LegalText, { text })]) })
      expect(wrapper.element.textContent).toBe(text)
      const links = wrapper.findAll('a')
      expect(links).toHaveLength(text.split('support@trachuan.vn').length - 1)
      for (const link of links) expect(link.attributes('href')).toBe('mailto:support@trachuan.vn')
    }
  }
})
