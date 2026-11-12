import type { components } from '@trachuan/schema/gen/openapi'

type Unit = components['schemas']['Unit']

export function joinedUnits(units: Unit[]): Unit[] {
  const result: Unit[] = []
  for (let i = 0; i < units.length; i++) {
    const unit = units[i]!
    if (!unit.parts || !unit.part) {
      result.push(unit)
      continue
    }
    if (unit.part !== 1) continue
    const parts = units.slice(i, i + unit.parts)
    if (parts.length !== unit.parts || parts.some((part, index) => part.part !== index + 1 || part.parts !== unit.parts || part.kind !== unit.kind || part.anchor !== unit.anchor)) break
    const joined = { ...unit }
    if (unit.kind === 'outline') joined.heading = parts.map(part => part.heading || '').join('')
    else if (unit.kind === 'relation' && unit.relation) joined.relation = { ...unit.relation, evidence: parts.map(part => part.relation?.evidence || '').join('') }
    else joined.text = parts.map(part => part.text || '').join('')
    result.push(joined)
    i += unit.parts - 1
  }
  return result
}


export function warningsByTarget(units: Unit[]): Map<string, NonNullable<Unit['warnings']>> {
  const groups = new Map<string, NonNullable<Unit['warnings']>>()
  const seen = new Set<string>()
  for (const unit of units) {
    for (const warning of unit.warnings || []) {
      const identity = JSON.stringify([warning.kind, warning.key, warning.source, warning.target])
      if (seen.has(identity)) continue
      seen.add(identity)
      const group = groups.get(warning.target) || []
      group.push(warning)
      groups.set(warning.target, group)
    }
  }
  return groups
}
