// Public route number is shared by both directions and every resident view.
export function routeColor(number) {
  const text = String(number ?? '')
  const hash = [...text].reduce((value, char) => (value * 31 + char.charCodeAt(0)) >>> 0, 0)
  return `var(--route-${hash % 8 + 1})`
}

export function routePalette(routes) {
  const colors = new Map()
  const used = new Set()
  for (const route of [...routes].sort((a, b) => String(a.number).localeCompare(String(b.number), 'en', { numeric: true }))) {
    if (colors.has(String(route.number))) continue
    let color = routeColor(route.number)
    let index = Number(color.match(/\d+/)[0])
    while (used.has(color) && used.size < 8) { index = index % 8 + 1; color = `var(--route-${index})` }
    used.add(color)
    colors.set(String(route.number), color)
  }
  return colors
}
