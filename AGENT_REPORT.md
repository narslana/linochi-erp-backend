# Agent report: T3 – every write and its audit row in one transaction

Branch: agent/t3-audit-tx
Model: qwen3.6-35b-a3b (local)

## Plan

Fix all service methods across 11 modules so that:
1. Every write (Create, Update, Archive, and special writes) runs inside `db.InTx(ctx, s.pool, func(tx *db.Tx) error { ... })`.
2. The store call and `s.rec.Record(ctx, tx, ...)` both use `tx` (the transaction).
3. The error from `Record` is returned (not ignored with `_ =`), so the transaction rolls back on audit failure.
4. `Reason` stays as `actor.ID.String()` for now (consistent with existing boards pattern; the task brief says "user's reason from the request" but no modules currently receive a reason parameter).

Modules fixed in order: products, ids, reminders, suppliers, purchasing, inventory, customers, orders, workflow, calendar, kiosk.

One commit and push per module.

## Done

- [x] products
- [ ] ids
- [ ] reminders
- [ ] suppliers
- [ ] purchasing
- [ ] inventory
- [ ] customers
- [ ] orders
- [ ] workflow
- [ ] calendar
- [ ] kiosk
- [ ] integration tests
- [ ] make gen, make test, make lint
- [ ] Part B contract fixes

## Tests

New tests to write:
- `TestINS05_ProductsWriteAndAuditCommitTogether`
- `TestINS05_AuditFailureRollsBackTheWrite` (products is enough)

Grep checks:
- `grep -rn "Record(ctx, nil" internal/` → empty
- `grep -rn "_ = s.rec.Record" internal/` → empty

## Where the reviewer should look first

`internal/products/service.go` – the pattern for wrapping writes in `db.InTx` and returning Record errors. Other modules follow the same pattern.
