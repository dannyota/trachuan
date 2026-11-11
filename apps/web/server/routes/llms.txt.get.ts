export default defineEventHandler((event) => {
  setResponseHeader(event, 'Content-Type', 'text/plain; charset=utf-8')
  const endpoint = `${useRuntimeConfig(event).public.siteOrigin.replace(/\/$/, '')}/mcp`
  return `Tra Chuẩn (trachuan) provides evidence from Vietnamese legal texts.\nMCP endpoint: ${endpoint}\n\nSearch, then scan documents and hits. Read chosen handles together, using a path anchor to read an article or chapter. Check status, warnings and incoming repeals, replaces, suspends and amends_supplements pointers. Read incoming implements documents for detailed rules. Follow next until the last page.\nA snippet is not the provision; […] marks omitted text. Pointer direction is as recorded by the source. Do not replace a pointer outside the release with a number lookup. Cite only complete returned text and cite the returned evidence in answers.\n\nPages: /connect, /sources, /privacy, /terms\n`
})
