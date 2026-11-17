type FixtureRoute = {
  request: () => { url: () => string }
  fetch: (options: { url: string }) => Promise<{ status: () => number, headers: () => Record<string, string>, body: () => Promise<Buffer> }>
  fulfill: (options: { status: number, headers: Record<string, string>, body: Buffer }) => Promise<void>
}

export async function fulfillFrameworkFixture(route: FixtureRoute, page: { isClosed: () => boolean }): Promise<void> {
  if (page.isClosed()) return
  try {
    const url = new URL(route.request().url())
    const response = await route.fetch({ url: `http://127.0.0.1:3032${url.pathname}${url.search}` })
    const status = response.status()
    const headers = response.headers()
    const body = await response.body()
    if (!page.isClosed()) await route.fulfill({ status, headers, body })
  } catch (error) {
    if (!page.isClosed()) throw error
  }
}
