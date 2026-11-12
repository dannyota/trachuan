import { APP_CSP } from '../../app/utils/csp'
import { matchPage } from '../utils/page-route'
import { admitPage } from '../utils/page-admission'

export default defineNitroPlugin((nitro) => {
  nitro.hooks.hook('request', (event) => {
    const response = event.node.res
    const isPublic = matchPage(event.node.req.url ?? '')?.public === true
    const finalize = () => {
      if (response.headersSent) return
      response.setHeader('Content-Security-Policy', APP_CSP)
      response.setHeader('Cache-Control', isPublic ? 'no-cache' : 'no-store')
      if (!isPublic) return
      response.removeHeader('Last-Modified')
      if (response.statusCode >= 400) response.removeHeader('ETag')
      const vary = response.getHeader('Vary')
      const values = (Array.isArray(vary) ? vary.join(',') : String(vary ?? ''))
        .split(',').map(value => value.trim()).filter(Boolean)
      if (!values.some(value => value.toLowerCase() === 'cookie')) values.push('Cookie')
      response.setHeader('Vary', values.join(', '))
    }
    response.end = new Proxy(response.end, {
      apply(target, receiver, args) {
        finalize()
        return Reflect.apply(target, receiver, args)
      },
    })
    response.writeHead = new Proxy(response.writeHead, {
      apply(target, receiver, args) {
        const index = typeof args[1] === 'string' ? 2 : 1
        const headers = args[index]
        if (headers) {
          const entries = Array.isArray(headers)
            ? Array.from({ length: headers.length / 2 }, (_, i) => [headers[2 * i], headers[2 * i + 1]])
            : Object.entries(headers)
          for (const [name, value] of entries) response.setHeader(name, value)
          args[index] = undefined
        }
        finalize()
        return Reflect.apply(target, receiver, args)
      },
    })
    admitPage(event)
  })
})
