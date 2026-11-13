import { defineStore } from 'pinia'

export const useSearchDraftStore = defineStore('search-draft', {
  state: (): { draft: { query: string, inForceOnly: boolean } | null } => ({ draft: null }),
})
