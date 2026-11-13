import type { components } from '@trachuan/schema/gen/openapi'
import type { Language } from '../../i18n'
import { relationLabel } from './relations'

type Pointer = components['schemas']['Pointer']
type ListedRelation = components['schemas']['ListedRelation']
export type RelationGroup = { label: string, dir: string | undefined, kind: string | undefined, items: Pointer[], more: number }

export function relationGroups(pointers: Pointer[] | undefined, counts: Record<string, number> | undefined, lang: Language): RelationGroup[] {
  const groups = new Map<string, RelationGroup>()
  const keys = new Map<string, Set<string>>()
  for (const pointer of pointers || []) {
    const label = relationLabel(pointer.kind, pointer.dir, lang)
    let group = groups.get(label)
    if (!group) {
      group = { label, dir: pointer.dir, kind: pointer.kind, items: [], more: 0 }
      groups.set(label, group)
      keys.set(label, new Set())
    }
    group.items.push(pointer)
    keys.get(label)!.add(`${pointer.dir || ''}:${pointer.kind || ''}`)
  }
  for (const [label, group] of groups) {
    let total = 0
    for (const key of keys.get(label)!) total += counts?.[key] ?? group.items.filter(item => `${item.dir || ''}:${item.kind || ''}` === key).length
    group.more = Math.max(0, total - group.items.length)
  }
  return [...groups.values()]
}

export function legalBases(relations: ListedRelation[]): ListedRelation[] {
  return relations.filter(relation => relation.dir === 'out' && relation.kind === 'legal_basis')
}
