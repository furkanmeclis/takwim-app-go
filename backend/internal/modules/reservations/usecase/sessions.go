package usecase

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	availabilityusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
)

// Package sessions of a visit (TEC-920, requirements §13 "Çok seanslı
// paket", A19):
//
//   - A line of an open visit can use one session of a package of the
//     visit's customer for the same service: active, not refunded, valid
//     on the visit day and usable in the branch. The package row is
//     locked and the lowest free session number taken, so the last
//     session is never given to two visits (a unique index backs it).
//   - A line on a session is paid by the package: the visit's total
//     leaves it out.
//   - Completing the visit consumes its sessions; the completion hook
//     (sales) writes the line at the session's value, without charging
//     the customer again.
//   - Cancel, reject, expiry and no-show release the sessions; removing
//     it from the visit too. A move keeps a session while its line keeps
//     the service and the new day is inside the package's validity.

var (
	// ErrSessionNotFound: the package session is not part of the visit.
	ErrSessionNotFound = errors.New("package session not found")
)

// releaseSessions frees every session of the visit that was not used
// (reject, cancel, expiry, no-show).
func releaseSessions(ctx context.Context, q *db.Queries, orgID, reservationID int64, reason string) error {
	_, err := q.ReleaseReservationPackageSessions(ctx, db.ReleaseReservationPackageSessionsParams{
		OrganizationID: orgID, ReservationID: reservationID, ReleaseReason: pgtype.Text{String: reason, Valid: true},
	})
	return err
}

// consumeSessions uses the held sessions of a completed visit.
func consumeSessions(ctx context.Context, q *db.Queries, orgID, reservationID int64, now time.Time) error {
	_, err := q.ConsumeReservationPackageSessions(ctx, db.ConsumeReservationPackageSessionsParams{
		OrganizationID: orgID, ReservationID: reservationID, Now: ts(now),
	})
	return err
}

// reconcileSessions runs after a move rewrote the lines: a held session
// stays while its line keeps the package's service and the new day is
// inside the package's validity; otherwise it is released (rescheduled).
func reconcileSessions(ctx context.Context, q *db.Queries, orgID, reservationID int64) error {
	held, err := q.ListHeldReservationPackageSessions(ctx, db.ListHeldReservationPackageSessionsParams{
		OrganizationID: orgID, ReservationID: reservationID,
	})
	if err != nil || len(held) == 0 {
		return err
	}
	day, err := q.GetPackageVisitDay(ctx, db.GetPackageVisitDayParams{OrganizationID: orgID, ReservationID: reservationID})
	if err != nil {
		return err
	}
	for _, h := range held {
		line, err := q.GetPackageLine(ctx, db.GetPackageLineParams{OrganizationID: orgID, ReservationID: reservationID, LineIndex: h.LineIndex})
		keep := err == nil && line.ServiceID.Valid && h.PackageServiceID.Valid && line.ServiceID.Int64 == h.PackageServiceID.Int64 &&
			(!h.ExpiresOn.Valid || !day.Time.After(h.ExpiresOn.Time))
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if keep {
			continue
		}
		if err := q.ReleasePackageSession(ctx, db.ReleasePackageSessionParams{
			OrganizationID: orgID, ID: h.ID, ReleaseReason: pgtype.Text{String: "rescheduled", Valid: true},
		}); err != nil {
			return err
		}
	}
	return nil
}

// ApplyPackage gives a session of one of the customer's packages to a
// line of an open visit; the visit's total leaves the line out.
func (s *Service) ApplyPackage(ctx context.Context, orgID int64, id uuid.UUID, caller availabilityusecase.Caller, actorID int64, in model.ApplyPackageInput) (model.Reservation, error) {
	return s.changeProducts(ctx, orgID, id, caller, actorID, in.ExpectedVersion, "package_applied", func(tx *availabilityusecase.Tx, row db.Reservation) (string, error) {
		q := tx.Q
		ve := &ValidationError{}
		line, err := q.GetPackageLine(ctx, db.GetPackageLineParams{OrganizationID: orgID, ReservationID: row.ID, LineIndex: int32(in.LineIndex)})
		if errors.Is(err, pgx.ErrNoRows) {
			ve.add("line_index", "not_found", "the visit has no such line")
			return "", ve
		}
		if err != nil {
			return "", err
		}
		pkg, err := q.GetCustomerPackageRow(ctx, db.GetCustomerPackageRowParams{OrganizationID: orgID, Uuid: in.CustomerPackageUUID})
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && pkg.CustomerID != row.CustomerID) {
			ve.add("customer_package_uuid", "not_found", "package not found")
			return "", ve
		}
		if err != nil {
			return "", err
		}
		// Serialize the sessions of one package.
		pkg, err = q.LockCustomerPackage(ctx, db.LockCustomerPackageParams{OrganizationID: orgID, ID: pkg.ID})
		if err != nil {
			return "", err
		}
		day, err := q.GetPackageVisitDay(ctx, db.GetPackageVisitDayParams{OrganizationID: orgID, ReservationID: row.ID})
		if err != nil {
			return "", err
		}
		live, err := q.LivePackageSessionNumbers(ctx, db.LivePackageSessionNumbersParams{OrganizationID: orgID, CustomerPackageID: pkg.ID})
		if err != nil {
			return "", err
		}
		switch {
		case pkg.Status != "active":
			ve.add("customer_package_uuid", "refunded", "the package was refunded")
		case pkg.ExpiresOn.Valid && day.Time.After(pkg.ExpiresOn.Time):
			ve.add("customer_package_uuid", "expired", "the package is not valid on the visit day")
		case pkg.BranchIds != nil && !slices.Contains(pkg.BranchIds, row.BranchID):
			ve.add("customer_package_uuid", "branch_not_allowed", "the package cannot be used in this branch")
		case !line.ServiceID.Valid || !pkg.ServiceID.Valid || line.ServiceID.Int64 != pkg.ServiceID.Int64:
			ve.add("line_index", "service_mismatch", "the package is for another service")
		case len(live) >= int(pkg.TotalSessions):
			ve.add("customer_package_uuid", "no_sessions_left", "the package has no session left")
		}
		if err := ve.orNil(); err != nil {
			return "", err
		}
		no := int32(1)
		for _, n := range live {
			if n != no {
				break
			}
			no++
		}
		amount := model.SessionValue(money.FromNumericOrZero(pkg.Price), int(pkg.TotalSessions), int(no))
		if _, err := q.InsertPackageSession(ctx, db.InsertPackageSessionParams{
			OrganizationID: orgID, CustomerPackageID: pkg.ID, ReservationID: row.ID, LineIndex: line.LineIndex,
			SessionNo: no, Amount: amount.Numeric(), CreatedBy: pgtype.Int8{Int64: actorID, Valid: actorID > 0},
		}); err != nil {
			if isUnique(err, "uq_package_sessions_line") {
				ve.add("line_index", "already_on_package", "the line already uses a package session")
				return "", ve
			}
			if isUnique(err, "uq_package_sessions_slot") {
				ve.add("customer_package_uuid", "no_sessions_left", "the package has no session left")
				return "", ve
			}
			return "", err
		}
		return fmt.Sprintf("%s · %s %d/%d", line.ServiceName, pkg.Name, no, pkg.TotalSessions), nil
	})
}

// RemovePackageSession takes a held session off the visit; the line is
// paid at its booked price again.
func (s *Service) RemovePackageSession(ctx context.Context, orgID int64, id, sessionID uuid.UUID, caller availabilityusecase.Caller, actorID int64, in ChangeInput) (model.Reservation, error) {
	return s.changeProducts(ctx, orgID, id, caller, actorID, in.ExpectedVersion, "package_released", func(tx *availabilityusecase.Tx, row db.Reservation) (string, error) {
		ps, err := tx.Q.GetReservationPackageSessionForUpdate(ctx, db.GetReservationPackageSessionForUpdateParams{
			OrganizationID: orgID, ReservationID: row.ID, Uuid: sessionID,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrSessionNotFound
		}
		if err != nil {
			return "", err
		}
		if ps.Status != model.SessionHeld {
			return "", ErrInvalidTransition
		}
		if err := tx.Q.ReleasePackageSession(ctx, db.ReleasePackageSessionParams{
			OrganizationID: orgID, ID: ps.ID, ReleaseReason: pgtype.Text{String: "removed", Valid: true},
		}); err != nil {
			return "", err
		}
		return fmt.Sprintf("%s %d", ps.PackageName, ps.SessionNo), nil
	})
}

// PackageOptions lists the customer's packages that can give a session to
// the visit's lines.
func (s *Service) PackageOptions(ctx context.Context, orgID int64, id uuid.UUID, caller availabilityusecase.Caller) ([]model.PackageOption, error) {
	row, err := s.visible(ctx, s.q, orgID, id, caller, false)
	if err != nil {
		return nil, err
	}
	out := []model.PackageOption{}
	if !productsOpen(row.Status) {
		return out, nil
	}
	day, err := s.q.GetPackageVisitDay(ctx, db.GetPackageVisitDayParams{OrganizationID: orgID, ReservationID: row.ID})
	if err != nil {
		return nil, err
	}
	rows, err := s.q.ReservationPackageOptions(ctx, db.ReservationPackageOptionsParams{
		OrganizationID: orgID, CustomerID: row.CustomerID, BranchID: row.BranchID, VisitDay: day,
	})
	if err != nil {
		return nil, err
	}
	lines, err := s.q.ListReservationLines(ctx, db.ListReservationLinesParams{OrganizationID: orgID, ReservationID: row.ID})
	if err != nil {
		return nil, err
	}
	sessions, err := s.q.ListReservationPackageSessions(ctx, db.ListReservationPackageSessionsParams{OrganizationID: orgID, ReservationID: row.ID})
	if err != nil {
		return nil, err
	}
	covered := map[int32]bool{}
	for _, ps := range sessions {
		if ps.Status != model.SessionReleased {
			covered[ps.LineIndex] = true
		}
	}
	for _, r := range rows {
		left := int(r.TotalSessions) - int(r.LiveCount)
		if left <= 0 {
			continue
		}
		o := model.PackageOption{
			CustomerPackageUUID: r.Uuid, Name: r.Name, ServiceName: r.ServiceName, TotalSessions: int(r.TotalSessions),
			Remaining: left, LineIndexes: []int{},
			NextValue: model.SessionValue(money.FromNumericOrZero(r.Price), int(r.TotalSessions), int(r.LiveCount)+1),
		}
		if r.ServiceUuid.Valid {
			u := uuid.UUID(r.ServiceUuid.Bytes)
			o.ServiceUUID = &u
		}
		if r.ExpiresOn.Valid {
			d := r.ExpiresOn.Time.Format("2006-01-02")
			o.ExpiresOn = &d
		}
		for _, l := range lines {
			if !covered[l.LineIndex] && r.ServiceID.Valid && l.ServiceID.Valid && l.ServiceID.Int64 == r.ServiceID.Int64 {
				o.LineIndexes = append(o.LineIndexes, int(l.LineIndex))
			}
		}
		out = append(out, o)
	}
	return out, nil
}

// listSessions reads the package sessions of a visit.
func listSessions(ctx context.Context, q *db.Queries, orgID, reservationID int64) ([]model.PackageSession, error) {
	rows, err := q.ListReservationPackageSessions(ctx, db.ListReservationPackageSessionsParams{OrganizationID: orgID, ReservationID: reservationID})
	if err != nil {
		return nil, err
	}
	out := make([]model.PackageSession, 0, len(rows))
	for _, r := range rows {
		ps := model.PackageSession{
			UUID: r.Uuid, LineIndex: int(r.LineIndex), CustomerPackage: model.Ref{UUID: r.CustomerPackageUuid, Name: r.PackageName},
			SessionNo: int(r.SessionNo), TotalSessions: int(r.TotalSessions), Amount: money.FromNumericOrZero(r.Amount),
			Status: r.Status, ReleaseReason: textPtr(r.ReleaseReason), ConsumedAt: tsPtr(r.ConsumedAt),
			ReleasedAt: tsPtr(r.ReleasedAt), CreatedAt: r.CreatedAt.Time.UTC(),
		}
		out = append(out, ps)
	}
	return out, nil
}

// isUnique reports a violation of the named unique index.
func isUnique(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == constraint
}
