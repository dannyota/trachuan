import type { components } from '@trachuan/schema/gen/openapi'
import type { Language } from '../../i18n'
import { relationLabel } from './relations'

type Pointer = components['schemas']['Pointer']
type ListedRelation = components['schemas']['ListedRelation']
export type RelationGroup = { label: string, dir: string | undefined, kind: string | undefined, items: Pointer[], more: number }

const kindOrder = ['repeals', 'replaces', 'suspends', 'amends_supplements', 'consolidates', 'implements', 'guided_by', 'legal_basis', 'cites', 'references', 'unknown']

export function relationGroups(pointers: Pointer[] | undefined, counts: Record<string, number> | undefined, lang: Language): RelationGroup[] {
  const groups = new Map<string, RelationGroup>()
  function groupFor(dir: string | undefined, kind: string | undefined): RelationGroup {
    const key = `${dir || ''}:${kind || ''}`
    let group = groups.get(key)
    if (!group) {
      group = { label: relationLabel(kind, dir, lang), dir, kind, items: [], more: 0 }
      groups.set(key, group)
    }
    return group
  }
  for (const [key, count] of Object.entries(counts || {})) {
    const parts = key.split(':')
    if (parts.length !== 2 || !parts[0] || !parts[1] || count <= 0) continue
    groupFor(parts[0], parts[1]).more = count
  }
  for (const pointer of pointers || []) groupFor(pointer.dir, pointer.kind).items.push(pointer)
  for (const [key, group] of groups) group.more = Math.max(0, (counts?.[key] ?? group.items.length) - group.items.length)
  return [...groups.values()].sort((a, b) => {
    const aIndex = kindOrder.indexOf(a.kind || ''), bIndex = kindOrder.indexOf(b.kind || '')
    const aRank = aIndex < 0 ? kindOrder.length : aIndex, bRank = bIndex < 0 ? kindOrder.length : bIndex
    const dirRank = (dir: string | undefined) => dir === 'in' ? 0 : dir === 'out' ? 1 : 2
    return aRank - bRank || dirRank(a.dir) - dirRank(b.dir) || (a.kind || '').localeCompare(b.kind || '') || (a.dir || '').localeCompare(b.dir || '')
  })
}

export function legalBases(relations: ListedRelation[]): ListedRelation[] {
  return relations.filter(relation => relation.dir === 'out' && relation.kind === 'legal_basis')
}
