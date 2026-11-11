import type { vi } from './vi'

type Catalog = { [K in keyof typeof vi]: K extends 'errors' ? { [E in keyof typeof vi.errors]: string } : string }
export const en: Catalog = {
  signIn: 'Sign in',
  account: 'Account',
  language: 'Tiếng Việt',
  home: 'Legal research',
  connect: 'Connect',
  sources: 'Sources',
  privacy: 'Privacy',
  terms: 'Terms',
  login: 'Sign in',
  errors: {
    invalid_request: 'The request is invalid.',
    unauthenticated: 'Please sign in.',
    csrf_rejected: 'The security session is invalid. Please try again.',
    account_blocked: 'The account is blocked.',
    not_found: 'The requested item was not found.',
    release_expired: 'The release has expired.',
    body_too_large: 'The request is too large.',
    unsupported_media_type: 'The format is not supported.',
    token_limit: 'The token limit has been reached.',
    rate_limited: 'Too many requests. Please try again later.',
    busy: 'The service is busy. Please try again later.',
    unavailable: 'The service is unavailable.',
    inference_failed: 'The request could not be processed.',
    internal: 'Something went wrong. Please try again.',
    generic: 'Something went wrong. Please try again.',
  },
}
