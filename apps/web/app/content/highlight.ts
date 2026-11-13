type Token = { start: number, end: number, value: string }

function fold(text: string): string {
  return text.normalize('NFC').split('ı').map(part => part.toUpperCase().toLowerCase().replaceAll('ß', 'ss').replaceAll('ς', 'σ')).join('ı').normalize('NFC')
}

function tokens(text: string): Token[] {
  return [...text.matchAll(/[^\p{White_Space}\p{P}]+/gu)].map(match => ({
    start: match.index,
    end: match.index + match[0].length,
    value: fold(match[0]),
  }))
}

export function highlightSpans(text: string, query: string): Array<{ start: number, end: number }> {
  const source = tokens(text)
  const target = tokens(query)
  const spans: Array<{ start: number, end: number }> = []
  for (let i = 0; i < source.length;) {
    let best = 0
    for (let j = 0; j < target.length; j++) {
      let length = 0
      while (i + length < source.length && j + length < target.length && source[i + length]!.value === target[j + length]!.value) length++
      best = Math.max(best, length)
    }
    if (best >= 2 || (best === 1 && /\p{N}/u.test(source[i]!.value))) {
      spans.push({ start: source[i]!.start, end: source[i + best - 1]!.end })
      i += best
    } else {
      i++
    }
  }
  return spans
}
