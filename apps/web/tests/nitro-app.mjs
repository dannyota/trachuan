const exports = await import('../.output/server/chunks/nitro/nitro.mjs')
const useNitroApp = Object.values(exports).find(value => typeof value === 'function' && value.name === 'useNitroApp')
if (!useNitroApp) throw new Error('Built Nitro does not export useNitroApp')
export const nitroApp = useNitroApp()
