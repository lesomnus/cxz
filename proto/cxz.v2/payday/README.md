# Generated

Everything in this directory is written by `pd gen`, and editing it lasts
until the next one.

- `*.proto` are payday's own entities, copied in whole. They are copied
  rather than imported because everything generated from them has to be one set
  of types in one Go package: an ent schema in another package cannot have an
  edge to one here, and the wall between tenants is an edge.
- `*_svc.g.proto` are the service contracts generated from them, the same
  as the ones beside your own entities.

To change what payday's entities hold, write an overlay in `proto/ext/payday/`
-- one file per entity, named after it, and it is merged in here on the next
generation.
