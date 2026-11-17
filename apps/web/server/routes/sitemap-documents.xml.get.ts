import { sitemap } from '../utils/sitemap'

export default defineEventHandler(event => sitemap(event, 'documents'))
