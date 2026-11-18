package workflow

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/inventory"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/db"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/db/dbgen"
)

type plan struct {
	from, to Step
	effects  []PlannedEffect
	optional bool // some reserve effects are best effort (reopen)
}

// plan checks that the order may move from its step to toKey and works out what
// that does to stock. It only reads. Which effects there are follows from the
// stock phases of the two steps and from what the ledger says the order holds,
// so an override forwards or backwards needs no special cases per step name.
func (s *Service) plan(ctx context.Context, q *dbgen.Queries, fromKey string, versionID, orderID uuid.UUID, toKey string, override bool) (*plan, error) {
	steps, err := q.ListWorkflowSteps(ctx, versionID)
	if err != nil {
		return nil, fmt.Errorf("list workflow steps: %w", err)
	}
	byKey := map[string]Step{}
	for _, st := range steps {
		byKey[st.Key] = Step{ID: st.ID, StepDef: stepDef(st)}
	}
	from, ok := byKey[fromKey]
	if !ok {
		return nil, fmt.Errorf("order status %q is not a step of its workflow version %s", fromKey, versionID)
	}
	to, ok := byKey[toKey]
	if !ok || to.Key == from.Key {
		return nil, &TransitionNotAllowedError{From: from.Key, To: toKey}
	}
	if from.IsLocked {
		return nil, &FinalStateError{Key: from.Key}
	}
	reopen := false
	if override {
		// An override may go to any non-final step. Leaving a final step that is
		// not locked (cancelled, rejected) is a reopen.
		if to.IsFinal {
			return nil, &TransitionNotAllowedError{From: from.Key, To: toKey}
		}
		reopen = from.IsFinal
	} else {
		trs, err := q.ListWorkflowTransitions(ctx, versionID)
		if err != nil {
			return nil, fmt.Errorf("list workflow transitions: %w", err)
		}
		allowed := false
		for _, tr := range trs {
			if tr.FromKey == from.Key && tr.ToKey == to.Key {
				allowed = true
				break
			}
		}
		if !allowed {
			return nil, &TransitionNotAllowedError{From: from.Key, To: toKey}
		}
	}

	lines, err := q.ListOrderLinesForEngine(ctx, orderID)
	if err != nil {
		return nil, fmt.Errorf("list order lines: %w", err)
	}
	p := &plan{from: from, to: to}
	var returns, releases, reserves, ships []PlannedEffect
	fromP, toP := phaseOf(from.StepDef), phaseOf(to.StepDef)
	for _, l := range lines {
		if l.Quantity <= 0 {
			continue
		}
		qty := int(l.Quantity)
		reserved, err := q.ReservedByLocationForLine(ctx, &l.ID)
		if err != nil {
			return nil, fmt.Errorf("reserved for line: %w", err)
		}
		sold, err := q.SoldByBatchForLine(ctx, &l.ID)
		if err != nil {
			return nil, fmt.Errorf("sold for line: %w", err)
		}
		var reservedQty, soldQty int
		for _, r := range reserved {
			reservedQty += int(r.Reserved)
		}
		for _, r := range sold {
			soldQty += int(r.Sold)
		}
		eff := func(kind string, n int) PlannedEffect {
			return PlannedEffect{Kind: kind, OrderLineID: l.ID, ProductID: l.ProductID, Quantity: n}
		}
		if fromP == PhaseShipped && toP != PhaseShipped && soldQty > 0 {
			returns = append(returns, eff(EffectReturn, soldQty))
		}
		switch toP {
		case PhaseNone:
			if (fromP != PhaseNone || to.IsFinal) && reservedQty > 0 {
				releases = append(releases, eff(EffectRelease, reservedQty))
				reservedQty = 0
			}
			// ASSUMPTION: reopening a cancelled or rejected order reserves its stock again
			// even though the target step (new) holds no reservation by phase; it is
			// best effort (owner decision 2026-11-16).
			if reopen && qty > reservedQty {
				r := eff(EffectReserve, qty-reservedQty)
				reserves = append(reserves, r)
				p.optional = true
			}
		case PhaseReserved:
			if fromP != PhaseReserved && qty > reservedQty {
				reserves = append(reserves, eff(EffectReserve, qty-reservedQty))
			}
		case PhaseShipped:
			if fromP != PhaseShipped && qty > soldQty {
				ships = append(ships, eff(EffectShip, qty-soldQty))
			}
		}
	}
	p.effects = append(append(append(returns, releases...), reserves...), ships...)
	return p, nil
}

// runEffects does what the plan says, in the caller's transaction. Any error
// aborts the whole status change.
func (s *Service) runEffects(ctx context.Context, tx *db.Tx, p *plan, res *Result) error {
	q := tx.Queries()
	skipOptional := false
	if p.optional {
		ok, err := s.canReserveAll(ctx, q, p.effects)
		if err != nil {
			return err
		}
		if !ok {
			skipOptional = true
			res.Warnings = append(res.Warnings, WarningStockNotAvailable)
		}
	}
	kept := p.effects[:0:0]
	for _, e := range p.effects {
		if skipOptional && e.Kind == EffectReserve {
			continue
		}
		kept = append(kept, e)
		var err error
		switch e.Kind {
		case EffectReturn:
			err = s.doReturn(ctx, tx, e)
		case EffectRelease:
			err = s.doRelease(ctx, tx, e)
		case EffectReserve:
			err = s.doReserve(ctx, tx, e)
		case EffectShip:
			err = s.doShip(ctx, tx, e)
		}
		if err != nil {
			return err
		}
	}
	res.Effects = kept
	return nil
}

func ref(e PlannedEffect, qty int, loc uuid.UUID) inventory.StockRef {
	return inventory.StockRef{ProductID: e.ProductID, LocationID: loc, Quantity: qty, ReferenceType: orderLineRef, ReferenceID: e.OrderLineID}
}

func (s *Service) doReturn(ctx context.Context, tx *db.Tx, e PlannedEffect) error {
	q := tx.Queries()
	sold, err := q.SoldByBatchForLine(ctx, &e.OrderLineID)
	if err != nil {
		return fmt.Errorf("sold for line: %w", err)
	}
	lineID := e.OrderLineID.String()
	total := 0
	for _, r := range sold {
		if _, err := s.inv.PostMovement(ctx, tx, inventory.MovementInput{
			ProductID: e.ProductID, LocationID: r.LocationID, BatchNumber: r.BatchNumber,
			Type: inventory.TypeReturn, Quantity: int(r.Sold), Reason: "order moved back from a shipped step",
			ReferenceType: orderLineRef, ReferenceID: &lineID,
		}); err != nil {
			return err
		}
		total += int(r.Sold)
	}
	if total > 0 {
		if err := q.AddOrderLineShipped(ctx, dbgen.AddOrderLineShippedParams{ID: e.OrderLineID, QuantityShipped: int32(-total)}); err != nil { //nolint:gosec // bounded by order quantity
			return fmt.Errorf("update quantity shipped: %w", err)
		}
	}
	return nil
}

func (s *Service) doRelease(ctx context.Context, tx *db.Tx, e PlannedEffect) error {
	reserved, err := tx.Queries().ReservedByLocationForLine(ctx, &e.OrderLineID)
	if err != nil {
		return fmt.Errorf("reserved for line: %w", err)
	}
	for _, r := range reserved {
		if err := s.inv.Release(ctx, tx, ref(e, int(r.Reserved), r.LocationID)); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) doReserve(ctx context.Context, tx *db.Tx, e PlannedEffect) error {
	loc, err := pickLocation(ctx, tx.Queries(), e.ProductID, e.Quantity)
	if err != nil {
		return err
	}
	return s.inv.Reserve(ctx, tx, ref(e, e.Quantity, loc))
}

func (s *Service) doShip(ctx context.Context, tx *db.Tx, e PlannedEffect) error {
	q := tx.Queries()
	var loc uuid.UUID
	reserved, err := q.ReservedByLocationForLine(ctx, &e.OrderLineID)
	if err != nil {
		return fmt.Errorf("reserved for line: %w", err)
	}
	if len(reserved) > 0 {
		loc = reserved[0].LocationID
	} else if loc, err = pickLocation(ctx, q, e.ProductID, e.Quantity); err != nil {
		return err
	}
	// ASSUMPTION: batch FEFO (no batch given), as the brief says.
	_, err = s.inv.Ship(ctx, tx, inventory.ShipInput{StockRef: ref(e, e.Quantity, loc), OrderLineID: e.OrderLineID})
	return err
}

// pickLocation chooses where to take stock from: the location with the most
// available, whether or not it is enough (the inventory service then refuses
// with its own 409). ASSUMPTION: an order line has no location of its own yet,
// so one location per line is chosen at reservation time.
func pickLocation(ctx context.Context, q *dbgen.Queries, product uuid.UUID, need int) (uuid.UUID, error) {
	loc, _, err := bestLocation(ctx, q, product, need, nil)
	return loc, err
}

// bestLocation returns the location with the most available and whether it has
// at least need. avail, if given, replaces the ledger (used to simulate several lines).
func bestLocation(ctx context.Context, q *dbgen.Queries, product uuid.UUID, need int, avail map[uuid.UUID]int) (uuid.UUID, bool, error) {
	if avail == nil {
		rows, err := q.BalanceByLocation(ctx, product)
		if err != nil {
			return uuid.Nil, false, fmt.Errorf("balance by location: %w", err)
		}
		avail = map[uuid.UUID]int{}
		for _, r := range rows {
			avail[r.LocationID] = int(r.InStock - r.Reserved)
		}
	}
	var best uuid.UUID
	bestAvail := -1 << 31
	for loc, a := range avail {
		if a > bestAvail || (a == bestAvail && loc.String() < best.String()) {
			best, bestAvail = loc, a
		}
	}
	if best == uuid.Nil {
		locs, err := q.ListLocations(ctx)
		if err != nil {
			return uuid.Nil, false, fmt.Errorf("list locations: %w", err)
		}
		if len(locs) == 0 {
			return uuid.Nil, false, &inventory.InsufficientStockError{Detail: "no stock location exists"}
		}
		return locs[0].ID, false, nil
	}
	return best, bestAvail >= need, nil
}

// canReserveAll simulates the optional reserve effects one after the other and
// says whether every one of them finds a location with enough stock.
func (s *Service) canReserveAll(ctx context.Context, q *dbgen.Queries, effects []PlannedEffect) (bool, error) {
	sim := map[uuid.UUID]map[uuid.UUID]int{}
	for _, e := range effects {
		if e.Kind != EffectReserve {
			continue
		}
		if sim[e.ProductID] == nil {
			rows, err := q.BalanceByLocation(ctx, e.ProductID)
			if err != nil {
				return false, fmt.Errorf("balance by location: %w", err)
			}
			sim[e.ProductID] = map[uuid.UUID]int{}
			for _, r := range rows {
				sim[e.ProductID][r.LocationID] = int(r.InStock - r.Reserved)
			}
		}
		loc, ok, err := bestLocation(ctx, q, e.ProductID, e.Quantity, sim[e.ProductID])
		if err != nil || !ok {
			return false, err
		}
		sim[e.ProductID][loc] -= e.Quantity
	}
	return true, nil
}
