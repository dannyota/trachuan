import { createAdmission } from '../utils/public-admission'
import { matchPage } from '../utils/page-route'

export default defineNitroPlugin((nitro) => {
  const admission = createAdmission(Date.now)
  const renders = new WeakMap<object, () => void>()
  nitro.hooks.hook('render:before', (context) => {
    const page = matchPage(context.event.node.req.url ?? '')
    const release = admission.render(page?.path.endsWith('/summary') === true)
    if (!release) {
      context.response = {
        statusCode: 429,
        headers: { 'Retry-After': '1', 'Content-Type': 'text/plain; charset=utf-8' },
        body: 'Too Many Requests\n',
      }
      return
    }
    renders.set(context.event.context, release)
  })
  nitro.hooks.hook('render:response', (_response, context) => {
    renders.get(context.event.context)?.()
    renders.delete(context.event.context)
  })
  nitro.hooks.hook('error', (_error, context) => {
    if (context.event) {
      renders.get(context.event.context)?.()
      renders.delete(context.event.context)
    }
  })
})
