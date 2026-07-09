# Service-routing preset catalog

`presets.json` is the embedded catalog of built-in service domain/subnet lists
(YouTube, Instagram, Telegram, Netflix, OpenAI, Steam, …) used by the **Routes
/ Маршруты** feature. keen-manager applies each preset through Keenetic's
native domain-routing stack (an `object-group fqdn` bound to a `dns-proxy
route`), so only flat domain and CIDR lists are needed.

## Provenance

The lists are derived from the [awg-manager](https://github.com/hoaxisr/awg-manager)
preset set (`internal/presets/defaults.json`), which is in turn compiled from
public sing-box geosite rule-sets (SagerNet, vernette/rulesets) and curated
domain lists (itdoginfo/allow-domains). Only the flat `engines.dns.domains` /
`subnets` / `subscriptionUrl` fields are kept; the sing-box `.srs` rule-set
references are dropped because the router-native routing engine consumes plain
FQDNs, not compiled rule-sets.

## Regenerating

```sh
# fetch the upstream catalog
curl -fsSL https://raw.githubusercontent.com/hoaxisr/awg-manager/master/internal/presets/defaults.json -o /tmp/awg-presets.json
# transform into keen-manager's simplified format
node tools/genpresets/transform.mjs /tmp/awg-presets.json internal/presets/data/presets.json
```

Review the diff before committing — upstream domain lists change over time.
