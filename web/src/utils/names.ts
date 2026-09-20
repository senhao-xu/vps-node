export function buildNameMap(items: { id: number; name: string }[]): Map<number, string> {
  return new Map(items.map((item) => [item.id, item.name]))
}

export function lookupName(map: Map<number, string>, id: number, prefix = '#'): string {
  const name = map.get(id)
  return name ?? `${prefix}${id}`
}
