import type { components } from '@trachuan/schema/gen/openapi'
import type { Language } from '../../i18n'
import { formatNumber } from '../content'

type Status = components['schemas']['CorpusStatus']
type NodePage = components['schemas']['NodePage']

export function summaryGraph(origin: string, summary: components['schemas']['PublicSummary']) {
  const document = summary.document
  return [{
    '@type': 'Legislation', url: `${origin}/vn/documents/${document.key}/summary`, name: document.title,
    legislationIdentifier: document.number, legislationType: document.type,
    ...(document.issued ? { legislationDate: document.issued } : {}),
    ...(document.issuer ? { legislationPassedBy: document.issuer } : {}),
    legislationJurisdiction: 'VN', inLanguage: 'vi',
    ...(document.source_url?.startsWith('https://') ? { sameAs: document.source_url } : {}),
  }]
}

export function homeGraph(origin: string, language: Language) {
  const organization = { '@id': `${origin}/#organization` }
  const person = { '@id': `${origin}/#danny` }
  return [
    { '@type': 'WebSite', name: 'Tra Chuẩn', url: origin, inLanguage: 'vi', publisher: organization },
    { '@type': 'Organization', ...organization, name: 'Tra Chuẩn', url: origin, logo: `${origin}/icon-512.png`, email: 'support@trachuan.vn', founder: person },
    { '@type': 'WebApplication', name: 'Tra Chuẩn', url: origin, applicationCategory: 'ReferenceApplication', operatingSystem: 'Web', description: language === 'vi' ? 'Tra Chuẩn tìm các đoạn trong văn bản pháp luật Việt Nam và đưa nguyên văn, kèm số hiệu và trạng thái hiệu lực. Tra Chuẩn không viết câu trả lời và không tư vấn pháp lý.' : 'Tra Chuẩn finds passages in Vietnamese legal documents and shows them word for word, with the document number and status. It does not write answers or give legal advice.' },
    { '@type': 'Person', ...person, name: 'Danny', url: 'https://aboutme.vn/danny', sameAs: 'https://aboutme.vn/danny', image: `${origin}/maker-avatar-640.webp` },
  ]
}

export function topicGraph(origin: string, page: NodePage) {
  return [
    { '@type': 'BreadcrumbList', itemListElement: (page.path ?? []).map((node, index) => ({ '@type': 'ListItem', position: index + 1, name: node.name, item: `${origin}/vn/topics/${node.code}` })) },
    { '@type': 'ItemList', itemListElement: (page.documents ?? []).map(document => ({ '@type': 'Legislation', name: document.title, legislationIdentifier: document.number, ...(document.type ? { legislationType: document.type } : {}), legislationJurisdiction: 'VN' })) },
  ]
}

export function sourcesGraph(origin: string, status: Status, language: Language) {
  const release = /^(\d{4})(\d{2})(\d{2})(?:_\d+)?$/.exec(status.release)
  const organization = { '@id': `${origin}/#organization`, name: 'Tra Chuẩn', url: origin }
  const counts = [status.documents, status.provisions, status.nodes].map(value => formatNumber(value, language))
  const description = language === 'vi' ? `Dữ liệu từ Cơ sở dữ liệu quốc gia về văn bản pháp luật và Cổng thông tin điện tử Chính phủ. Bản phát hành ${status.release} có ${counts[0]} văn bản, ${counts[1]} quy định và ${counts[2]} chủ đề.` : `Data from the national legal database and the Government Portal. Release ${status.release} has ${counts[0]} documents, ${counts[1]} provisions and ${counts[2]} topics.`
  return [{ '@type': 'Dataset', name: 'Tra Chuẩn', description, url: `${origin}/sources`, version: status.release, ...(release ? { dateModified: `${release[1]}-${release[2]}-${release[3]}` } : {}), inLanguage: 'vi', spatialCoverage: 'Việt Nam', creator: organization, publisher: organization, isBasedOn: [{ '@type': 'CreativeWork', name: 'Cơ sở dữ liệu quốc gia về văn bản pháp luật', url: 'https://vbpl.vn/' }, { '@type': 'CreativeWork', name: 'Cổng thông tin điện tử Chính phủ', url: 'https://chinhphu.vn/' }] }]
}
