type Token = { start: number, end: number, value: string, accented: boolean }

function fold(text: string): string {
  return text.normalize('NFC').split('ı').map(part => part.toUpperCase().toLowerCase().replaceAll('ß', 'ss').replaceAll('ς', 'σ')).join('ı').normalize('NFC')
}

function base(text: string): string {
  return text.normalize('NFD').replace(/\p{M}/gu, '').replaceAll('đ', 'd')
}

function matches(source: Token, target: Token): boolean {
  return target.accented
    ? source.value === target.value
    : base(source.value) === target.value
}

function tokens(text: string): Token[] {
  return [...text.matchAll(/[^\p{White_Space}\p{P}]+/gu)].map(match => ({
    start: match.index,
    end: match.index + match[0].length,
    value: fold(match[0]),
    accented: /[\p{M}đĐ]/u.test(match[0].normalize('NFD')),
  }))
}

export function highlightSpans(text: string, query: string): Array<{ start: number, end: number }> {
  const source = tokens(text)
  const target = tokens(query)
  const marked = source.map(() => false)
  for (let i = 0; i < source.length; i++) {
    for (let j = 0; j < target.length; j++) {
      let length = 0
      while (i + length < source.length && j + length < target.length && matches(source[i + length]!, target[j + length]!)) length++
      if (length >= 2 || (length === 1 && /\p{N}/u.test(source[i]!.value))) {
        for (let k = i; k < i + length; k++) marked[k] = true
      }
    }
  }
  const spans: Array<{ start: number, end: number }> = []
  for (let i = 0; i < source.length;) {
    if (!marked[i]) { i++; continue }
    const start = source[i]!.start
    while (i + 1 < source.length && marked[i + 1]) i++
    spans.push({ start, end: source[i]!.end })
    i++
  }
  return spans
}
