// transform.mjs — converts awg-manager's internal/presets/defaults.json into
// keen-manager's simplified, router-native preset catalog.
//
// keen-manager routes domains through Keenetic's native `object-group fqdn` +
// `dns-proxy route` mechanism (KeeneticOS 5.x "Маршруты/DNS"), so we only need
// the flat domain/subnet lists — the sing-box .srs rule-set references are
// dropped. Composite ("covers") presets are expanded into the union of their
// members' domains/subnets. Presets the router-native engine can't express
// (sing-box-only rule-sets, no flat domains) are omitted.
//
// Usage: node transform.mjs <in defaults.json> <out presets.json>
import { readFileSync, writeFileSync } from 'node:fs'

const [, , inPath, outPath] = process.argv
const src = JSON.parse(readFileSync(inPath, 'utf8'))

const icon = (slug) => (slug || '').replace(/^lucide-/, '')
const dnsOf = (p) => (p.engines && p.engines.dns) || {}

// Index every source preset by id so composite ("covers") presets can be
// expanded into the union of their members' flat domain/subnet lists — the
// upstream generator inlines the union for some composites but not all.
const srcById = new Map(src.map((p) => [p.id, p]))

function collect(p, seen = new Set()) {
  if (seen.has(p.id)) return { domains: [], subnets: [] }
  seen.add(p.id)
  const dns = dnsOf(p)
  const domains = new Set(Array.isArray(dns.domains) ? dns.domains : [])
  const subnets = new Set(Array.isArray(dns.subnets) ? dns.subnets : [])
  for (const cid of p.covers || []) {
    const child = srcById.get(cid)
    if (!child) continue
    const c = collect(child, seen)
    c.domains.forEach((d) => domains.add(d))
    c.subnets.forEach((s) => subnets.add(s))
  }
  return { domains: [...domains], subnets: [...subnets] }
}

const out = src
  .map((p) => {
    const dns = dnsOf(p)
    const { domains, subnets } = collect(p)
    const rec = {
      id: p.id,
      name: p.name,
      category: p.category || 'other',
      icon: icon(p.iconSlug),
      domains: domains.sort(),
      subnets: subnets,
    }
    if (p.notice) rec.notice = p.notice
    if (dns.subscriptionUrl) rec.subscription_url = dns.subscriptionUrl
    if (Array.isArray(p.covers) && p.covers.length) rec.covers = p.covers
    return rec
  })
  // Drop presets the router-native DNS engine can't express (singbox-only
  // rule-sets with no flat domains, subnets or subscription URL).
  .filter((p) => p.domains.length || p.subnets.length || p.subscription_url)

// Stable order: by category, then name.
const catOrder = ['social', 'media', 'ai', 'gaming', 'developer', 'cloud', 'block']
out.sort((a, b) => {
  const ca = catOrder.indexOf(a.category), cb = catOrder.indexOf(b.category)
  if (ca !== cb) return (ca < 0 ? 99 : ca) - (cb < 0 ? 99 : cb)
  return a.name.localeCompare(b.name, 'en')
})

writeFileSync(outPath, JSON.stringify(out, null, 1) + '\n')
const domains = out.reduce((n, p) => n + p.domains.length, 0)
const subnets = out.reduce((n, p) => n + p.subnets.length, 0)
console.log(`wrote ${out.length} presets, ${domains} domains, ${subnets} subnets -> ${outPath}`)
