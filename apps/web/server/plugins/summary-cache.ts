import { summaryCacheDriver } from '../utils/summary-cache'

export default defineNitroPlugin(() => {
  useStorage().mount('summary', summaryCacheDriver(Date.now))
})
