Cujgel schema docs/cujgel/citadel-link.schema.json at commit e9170dba

The schema and valid example are copied unchanged. Tests pin their SHA-256
digests and compare the handwritten Go validator's patterns, bounds and required
fields with the schema. Update the fixtures and validator together when adopting
a new schema revision.

`negative-cases.json` and `positive-cases.json` are shared rejection and acceptance
corpora read by both the Go and plugin card tests. Their inputs use placeholder
IDs and repeated hashes.
