export function useStructuredData(nodes: () => object[]) {
  useHead(() => {
    const graph = nodes()
    return { script: graph.length ? [{ type: 'application/ld+json', innerHTML: JSON.stringify({ '@context': 'https://schema.org', '@graph': graph }).replace(/</g, '\\u003c') }] : [] }
  })
}
