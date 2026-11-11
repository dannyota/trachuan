import type { components } from '@trachuan/schema/gen/openapi'

type Coverage = components['schemas']['CorpusStatus']

export async function getCoverage(fetchStatus: () => Promise<{ data?: Coverage, error?: unknown }>): Promise<Coverage | null> {
  try {
    const { data } = await fetchStatus()
    return data ?? null
  } catch {
    return null
  }
}
