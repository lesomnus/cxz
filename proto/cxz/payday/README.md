# Generated

Everything in this directory is written by `pd gen`, and editing it lasts
until the next one -- the past's lock apart, below.

- `*.proto` are payday's own entities, copied in whole. They are copied
  rather than imported because everything generated from them has to be one set
  of types in one Go package: an ent schema in another package cannot have an
  edge to one here, and the wall between tenants is an edge.
- `event.proto` and `fact.proto` are here when the app declares a past
  -- `option (payday.app) = {past: {event: 17, fact: 16}};` -- numbered as it
  declares, and go when the declaration does.
- `notification.proto` and `notification_receipt.proto` are here only
  when the app declares `option (payday.app) = {notify: {notification: 40, receipt: 41}};`.
  Their domains come from that declaration; the public notification contract is
  copied beside the internal records and disappears with them on opt-out.
- `archived.proto` and `access.proto` are at domains 5 and 6, or at the
  numbers the app gives them when its own entities hold those --
  `option (payday.app) = {domains: {archived: 240, access: 241}};`.
- `*_svc.g.proto` are the service contracts generated from them, the same
  as the ones beside your own entities.
- `past.lock`, with a past, is the record of every kind of event the app
  has declared and of the message its rows hold, and the one file here that
  is not written over: `pd gen` only adds to it, and refuses a schema that
  lost or changed what it holds -- a kind dropped or moved to another message
  among them -- since a row of an event is read back by name for as long as
  it is kept. Commit it. Taking something out of it by hand is right only for
  a type or a kind no deployment ever wrote a row of.

To change what payday's entities hold, write an overlay in `proto/ext/payday/`
-- one file per entity, named after it, and it is merged in here on the next
generation.
