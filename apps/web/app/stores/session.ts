import { defineStore } from 'pinia'
import type { components } from '@trachuan/schema/gen/openapi'

type Account = Omit<components['schemas']['Account'], 'csrf_token'>
export const useSessionStore = defineStore('session', {
  state: (): { account: Account | null, csrfToken: string | null, status: 'unknown' | 'signed-in' | 'signed-out' } => ({
    account: null,
    csrfToken: null,
    status: 'unknown',
  }),
})
