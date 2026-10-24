package adapters

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/cash/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/cash/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/i18n"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
)

// Engine slugs of the reconciliation.
const (
	StatementsResource     = model.StatementsSlug
	StatementLinesResource = model.StatementLinesSlug
)

// lineEntityType is the import rollback entity of a statement line.
const lineEntityType = "cash_statement_line"

// LineFilterFromQuery maps list query params (q, statement, status,
// line_type, direction).
func LineFilterFromQuery(query map[string]string) model.LineFilter {
	f := model.LineFilter{Q: strings.TrimSpace(query["q"])}
	for _, part := range apiquery.SplitCSV(query["statement"]) {
		if id, err := uuid.Parse(part); err == nil {
			f.Statements = append(f.Statements, id)
		}
	}
	f.Statuses = apiquery.SplitCSV(query["status"])
	f.LineTypes = apiquery.SplitCSV(query["line_type"])
	if d := strings.TrimSpace(query["direction"]); d == model.DirectionIn || d == model.DirectionOut {
		f.Direction = d
	}
	return f
}

// StatementLinesIO exports the lines of statements and imports a bank
// statement or provider settlement report into one statement: every row of
// the file becomes a line, matched with the book on the spot when one
// movement of the same amount sits within a few days.
//
// The sheet is read the way banks and providers write it: Turkish or
// English number formats ("1.250,50", "1,250.50", "-250,00 TL"), a signed
// amount column or separate credit / debit columns, a direction column in
// either language, several date layouts and Excel serial dates.
type StatementLinesIO struct {
	svc *usecase.Service
}

// NewStatementLinesIO builds the statement line io adapter.
func NewStatementLinesIO(svc *usecase.Service) *StatementLinesIO {
	return &StatementLinesIO{svc: svc}
}

func (a *StatementLinesIO) Resource() string { return StatementLinesResource }

func (a *StatementLinesIO) ExportColumns() []ioengine.Column {
	return []ioengine.Column{
		{Key: "statement", LabelKey: "cash_statement_line.statement", Type: ioengine.ColumnTypeString},
		{Key: "account", LabelKey: "cash_statement_line.account", Type: ioengine.ColumnTypeString},
		{Key: "line_no", LabelKey: "cash_statement_line.line_no", Type: ioengine.ColumnTypeString},
		{Key: "line_date", LabelKey: "cash_statement_line.line_date", Type: ioengine.ColumnTypeString},
		{Key: "direction", LabelKey: "cash_statement_line.direction", Type: ioengine.ColumnTypeEnum},
		{Key: "amount", LabelKey: "cash_statement_line.amount", Type: ioengine.ColumnTypeString},
		{Key: "fee", LabelKey: "cash_statement_line.fee", Type: ioengine.ColumnTypeString},
		{Key: "line_type", LabelKey: "cash_statement_line.line_type", Type: ioengine.ColumnTypeEnum},
		{Key: "description", LabelKey: "cash_statement_line.description", Type: ioengine.ColumnTypeString},
		{Key: "reference", LabelKey: "cash_statement_line.reference", Type: ioengine.ColumnTypeString},
		{Key: "balance_after", LabelKey: "cash_statement_line.balance_after", Type: ioengine.ColumnTypeString},
		{Key: "status", LabelKey: "cash_statement_line.status", Type: ioengine.ColumnTypeEnum},
		{Key: "match_method", LabelKey: "cash_statement_line.match_method", Type: ioengine.ColumnTypeEnum},
		{Key: "matched_movement", LabelKey: "cash_statement_line.matched_movement", Type: ioengine.ColumnTypeString},
		{Key: "difference_amount", LabelKey: "cash_statement_line.difference_amount", Type: ioengine.ColumnTypeString},
		{Key: "posted_movement", LabelKey: "cash_statement_line.posted_movement", Type: ioengine.ColumnTypeUUID},
		{Key: "note", LabelKey: "cash_statement_line.note", Type: ioengine.ColumnTypeString},
		{Key: "resolved_by", LabelKey: "cash_statement_line.resolved_by", Type: ioengine.ColumnTypeString},
		{Key: "uuid", LabelKey: "cash_statement_line.uuid", Type: ioengine.ColumnTypeUUID},
	}
}

func (a *StatementLinesIO) Export(ctx context.Context, query ioengine.ExportQuery, _ i18n.Locale) (ioengine.Dataset, error) {
	orgID, err := organizationID(ctx)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	rows, err := a.svc.ExportLines(ctx, orgID, LineFilterFromQuery(query))
	if err != nil {
		return ioengine.Dataset{}, err
	}
	out := make([]map[string]any, 0, len(rows))
	for _, l := range rows {
		row := map[string]any{
			"uuid": l.UUID.String(), "statement": l.Statement.UUID.String(), "account": l.Account.Name, "line_no": l.LineNo,
			"line_date": l.LineDate, "direction": l.Direction, "amount": l.Amount.String(), "fee": l.Fee.String(),
			"line_type": l.LineType, "description": l.Description, "reference": l.Reference, "balance_after": "",
			"status": l.Status, "match_method": l.MatchMethod, "matched_movement": "", "difference_amount": l.DifferenceAmount.String(),
			"posted_movement": "", "note": l.Note, "resolved_by": "",
		}
		if l.BalanceAfter != nil {
			row["balance_after"] = l.BalanceAfter.String()
		}
		if l.MatchedMovement != nil {
			desc := l.MatchedMovement.MovementDate
			if l.MatchedMovement.Amount != nil {
				desc += " · " + l.MatchedMovement.Amount.String()
			}
			if l.MatchedMovement.Description != "" {
				desc += " · " + l.MatchedMovement.Description
			}
			row["matched_movement"] = desc
		}
		if l.PostedMovement != nil {
			row["posted_movement"] = l.PostedMovement.UUID.String()
		}
		if l.ResolvedBy != nil {
			row["resolved_by"] = strings.TrimSpace(l.ResolvedBy.Name + " " + l.ResolvedBy.Surname)
		}
		out = append(out, row)
	}
	return ioengine.Dataset{Resource: StatementLinesResource, Columns: a.ExportColumns(), Rows: out}, nil
}

// ImportSchema is the statement sheet. statement_uuid comes from the
// import defaults (the wizard is opened from a statement); the amount is a
// signed column, or credit / debit columns, with an optional direction.
func (a *StatementLinesIO) ImportSchema() []ioengine.ImportField {
	return []ioengine.ImportField{
		{Key: "statement_uuid", LabelKey: "cash_statement_line.statement_uuid", Type: ioengine.ColumnTypeUUID, Required: true, DefaultHint: "statement"},
		{Key: "line_date", LabelKey: "cash_statement_line.line_date", Type: ioengine.ColumnTypeString, Required: true},
		{Key: "description", LabelKey: "cash_statement_line.description", Type: ioengine.ColumnTypeString},
		{Key: "amount", LabelKey: "cash_statement_line.amount", Type: ioengine.ColumnTypeString},
		{Key: "credit", LabelKey: "cash_statement_line.credit", Type: ioengine.ColumnTypeString},
		{Key: "debit", LabelKey: "cash_statement_line.debit", Type: ioengine.ColumnTypeString},
		{Key: "direction", LabelKey: "cash_statement_line.direction", Type: ioengine.ColumnTypeEnum},
		{Key: "fee", LabelKey: "cash_statement_line.fee", Type: ioengine.ColumnTypeString},
		{Key: "line_type", LabelKey: "cash_statement_line.line_type", Type: ioengine.ColumnTypeEnum},
		{Key: "reference", LabelKey: "cash_statement_line.reference", Type: ioengine.ColumnTypeString},
		{Key: "balance_after", LabelKey: "cash_statement_line.balance_after", Type: ioengine.ColumnTypeString},
	}
}

// SampleRows implements ioengine.SampleProvider: a bank sheet row with
// credit / debit columns and a provider settlement row with a fee.
func (a *StatementLinesIO) SampleRows() []map[string]any {
	return []map[string]any{
		{"line_date": "15.03.2026", "description": "POS tahsilatı", "credit": "1.250,00", "debit": "", "reference": "TRX-0001", "balance_after": "8.450,00"},
		{"line_date": "15.03.2026", "description": "Kira ödemesi", "credit": "", "debit": "4.500,00", "reference": "TRX-0002", "balance_after": "3.950,00"},
		{"line_date": "16.03.2026", "description": "Online ödeme", "amount": "890,00", "direction": "giriş", "fee": "17,80", "line_type": "ödeme", "reference": "PAY-77"},
		{"line_date": "17.03.2026", "description": "Hakediş aktarımı", "amount": "-872,20", "line_type": "hakediş", "reference": "PAYOUT-12"},
	}
}

// lineRow parses one sheet row (defaults fill empty cells).
func lineRow(row, defaults map[string]any) (uuid.UUID, model.LineInput, error) {
	get := func(key string) string {
		if v := cell(row, key); v != "" {
			return v
		}
		return cell(defaults, key)
	}
	var in model.LineInput
	stID, err := uuid.Parse(get("statement_uuid"))
	if err != nil {
		return uuid.Nil, in, errors.New("statement_uuid is required")
	}
	rawDate := get("line_date")
	if rawDate == "" {
		return stID, in, errors.New("line_date is required")
	}
	d, err := parseSheetDate(rawDate)
	if err != nil {
		return stID, in, fmt.Errorf("line_date %q is not a date", rawDate)
	}
	in.LineDate = d.Format("2006-01-02")
	in.Description = get("description")
	in.Reference = get("reference")
	direction, err := parseDirection(get("direction"))
	if err != nil {
		return stID, in, err
	}
	credit, debit, amount := get("credit"), get("debit"), get("amount")
	switch {
	case credit != "" || debit != "":
		c, err := parseOptionalAmount(credit)
		if err != nil {
			return stID, in, fmt.Errorf("credit %q is not an amount", credit)
		}
		d, err := parseOptionalAmount(debit)
		if err != nil {
			return stID, in, fmt.Errorf("debit %q is not an amount", debit)
		}
		switch {
		case !c.IsZero() && !d.IsZero():
			return stID, in, errors.New("a row has either a credit or a debit, not both")
		case !c.IsZero():
			in.Direction, in.Amount = model.DirectionIn, c
			if c.IsNegative() {
				in.Direction, in.Amount = model.DirectionOut, c.Neg()
			}
		case !d.IsZero():
			in.Direction, in.Amount = model.DirectionOut, d
			if d.IsNegative() {
				in.Direction, in.Amount = model.DirectionIn, d.Neg()
			}
		default:
			return stID, in, errors.New("credit or debit must be greater than zero")
		}
	case amount != "":
		m, err := parseSheetAmount(amount)
		if err != nil {
			return stID, in, fmt.Errorf("amount %q is not an amount", amount)
		}
		in.Amount = m
		switch {
		case m.IsNegative():
			in.Direction, in.Amount = model.DirectionOut, m.Neg()
		case direction != "":
			in.Direction = direction
		default:
			in.Direction = model.DirectionIn
		}
	default:
		return stID, in, errors.New("amount, credit or debit is required")
	}
	if raw := get("fee"); raw != "" {
		fee, err := parseSheetAmount(raw)
		if err != nil {
			return stID, in, fmt.Errorf("fee %q is not an amount", raw)
		}
		if fee.IsNegative() {
			fee = fee.Neg()
		}
		in.Fee = &fee
	}
	if raw := get("balance_after"); raw != "" {
		bal, err := parseSheetAmount(raw)
		if err != nil {
			return stID, in, fmt.Errorf("balance_after %q is not an amount", raw)
		}
		in.BalanceAfter = &bal
	}
	in.LineType = parseLineType(get("line_type"))
	return stID, in, nil
}

// PreviewRow implements ioengine.RowPreviewer: the dry run validates the
// row against its statement and tells whether it would match the book.
func (a *StatementLinesIO) PreviewRow(ctx context.Context, row map[string]any, defaults map[string]any) (ioengine.RowDiff, error) {
	orgID, err := organizationID(ctx)
	if err != nil {
		return ioengine.RowDiff{}, err
	}
	stID, in, err := lineRow(row, defaults)
	if err != nil {
		return ioengine.RowDiff{Error: err.Error()}, nil
	}
	p, err := a.svc.PreviewLine(ctx, orgID, stID, in)
	if err != nil {
		if rowErr(err) {
			return ioengine.RowDiff{Error: err.Error()}, nil
		}
		return ioengine.RowDiff{}, err
	}
	status := model.LinePending
	if p.Matched {
		status = model.LineMatched
	}
	changes := []ioengine.FieldChange{
		{Field: "line_date", To: p.Date},
		{Field: "direction", To: p.Direction},
		{Field: "amount", To: p.Amount},
		{Field: "line_type", To: p.Type},
		{Field: "status", To: status},
	}
	if p.Fee != "0.00" {
		changes = append(changes, ioengine.FieldChange{Field: "fee", To: p.Fee})
	}
	return ioengine.RowDiff{Op: "create", Changes: changes}, nil
}

// ApplyRow adds the row to its statement as a line.
func (a *StatementLinesIO) ApplyRow(ctx context.Context, row map[string]any, defaults map[string]any) (ioengine.RowResult, error) {
	orgID, err := organizationID(ctx)
	if err != nil {
		return ioengine.RowResult{}, err
	}
	stID, in, err := lineRow(row, defaults)
	if err != nil {
		return ioengine.RowResult{OK: false, Error: err.Error()}, nil
	}
	line, err := a.svc.AddLine(ctx, orgID, actorPtr(ctx), stID, in)
	if err != nil {
		if rowErr(err) {
			return ioengine.RowResult{OK: false, Error: err.Error()}, nil
		}
		return ioengine.RowResult{}, err
	}
	return ioengine.RowResult{
		OK: true, EntityType: lineEntityType, EntityUUID: line.UUID.String(), Op: "create",
		Previous: map[string]any{"status": line.Status},
	}, nil
}

// RevertRow removes an imported line that wrote nothing into the book.
func (a *StatementLinesIO) RevertRow(ctx context.Context, entityType, entityUUID string, _ map[string]any) error {
	if entityType != lineEntityType {
		return fmt.Errorf("unsupported entity %q", entityType)
	}
	orgID, err := organizationID(ctx)
	if err != nil {
		return err
	}
	id, err := uuid.Parse(entityUUID)
	if err != nil {
		return err
	}
	err = a.svc.RemoveLine(ctx, orgID, actorPtr(ctx), id)
	if errors.Is(err, usecase.ErrLineNotFound) {
		return nil
	}
	return err
}

// rowErr reports whether an error is the row's fault (reported on the
// row) rather than the system's (fails the job).
func rowErr(err error) bool {
	return usecase.IsValidation(err) ||
		errors.Is(err, usecase.ErrStatementNotFound) || errors.Is(err, usecase.ErrStatementClosed) ||
		errors.Is(err, usecase.ErrReferenceTaken) || errors.Is(err, usecase.ErrDayClosed) ||
		errors.Is(err, usecase.ErrAccountArchived) || errors.Is(err, usecase.ErrNotFound)
}

// actorPtr is the user a line is recorded for: the request principal, or
// the requester of the worker's job.
func actorPtr(ctx context.Context) *int64 {
	if p, ok := authctx.PrincipalFrom(ctx); ok {
		id := p.UserInternal
		return &id
	}
	if id, ok := orgctx.JobActor(ctx); ok {
		return &id
	}
	return nil
}

func cell(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(v))
}

// ----------------------------------------------------------- sheet parsing

var (
	amountNoise = regexp.MustCompile(`[^0-9,.\-+()]`)
	thousandsRE = regexp.MustCompile(`^\d{1,3}(\.\d{3})+$`)
	commaGroups = regexp.MustCompile(`^\d{1,3}(,\d{3})+$`)
	dateLayouts = []string{
		"2006-01-02", "02.01.2006", "02/01/2006", "02-01-2006", "2006/01/02", "2006.01.02",
		"02.01.2006 15:04", "02.01.2006 15:04:05", "02/01/2006 15:04", "2006-01-02 15:04", "2006-01-02 15:04:05",
		"2006-01-02T15:04:05Z07:00", "2006-01-02T15:04:05", "02.01.06", "02/01/06", "2 January 2006", "2 Jan 2006",
	}
)

// parseSheetAmount reads an amount the way a bank or a provider writes it:
// currency marks and spaces are dropped, "(250,00)" and "250,00-" are
// negative, and the decimal separator is the last of "," and "." (a lone
// "." followed by three digits groups thousands, Turkish style).
func parseSheetAmount(raw string) (money.Money, error) {
	s := strings.TrimSpace(raw)
	s = strings.NewReplacer(" ", "", " ", "", " ", "").Replace(s)
	neg := false
	if strings.HasPrefix(s, "(") && strings.HasSuffix(s, ")") {
		neg, s = true, strings.TrimSuffix(strings.TrimPrefix(s, "("), ")")
	}
	s = amountNoise.ReplaceAllString(s, "")
	if strings.HasSuffix(s, "-") {
		neg, s = true, strings.TrimSuffix(s, "-")
	}
	if strings.HasPrefix(s, "-") {
		neg, s = true, strings.TrimPrefix(s, "-")
	}
	s = strings.TrimPrefix(s, "+")
	if s == "" {
		return money.Money{}, money.ErrInvalid
	}
	comma, dot := strings.LastIndex(s, ","), strings.LastIndex(s, ".")
	switch {
	case comma >= 0 && dot >= 0:
		if comma > dot {
			s = strings.ReplaceAll(s, ".", "")
		} else {
			s = strings.ReplaceAll(s, ",", "")
		}
	case dot >= 0 && thousandsRE.MatchString(s):
		s = strings.ReplaceAll(s, ".", "")
	case comma >= 0 && commaGroups.MatchString(s) && strings.Count(s, ",") > 1:
		s = strings.ReplaceAll(s, ",", "")
	}
	m, err := money.ParseRound(s)
	if err != nil {
		return money.Money{}, err
	}
	if neg {
		m = m.Neg()
	}
	return m, nil
}

func parseOptionalAmount(raw string) (money.Money, error) {
	if strings.TrimSpace(raw) == "" || strings.TrimSpace(raw) == "-" {
		return money.Zero(), nil
	}
	return parseSheetAmount(raw)
}

// parseSheetDate reads the date layouts banks use and Excel serial dates.
func parseSheetDate(raw string) (time.Time, error) {
	s := strings.TrimSpace(raw)
	for _, layout := range dateLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC), nil
		}
	}
	// Excel stores dates as days since 1899-12-30 (serial 1 = 1900-01-01).
	if n, err := strconv.ParseFloat(strings.ReplaceAll(s, ",", "."), 64); err == nil && n >= 20000 && n < 80000 {
		return time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC).AddDate(0, 0, int(n)), nil
	}
	return time.Time{}, fmt.Errorf("not a date")
}

var (
	directionIn  = []string{"in", "giriş", "giris", "gelen", "alacak", "credit", "cr", "c", "+", "deposit", "yatan", "tahsilat", "gelir", "a"}
	directionOut = []string{"out", "çıkış", "cikis", "giden", "borç", "borc", "debit", "dr", "d", "-", "withdrawal", "çekilen", "cekilen", "ödeme", "odeme", "gider", "b"}
)

func parseDirection(raw string) (string, error) {
	v := strings.ToLower(strings.TrimSpace(raw))
	switch {
	case v == "":
		return "", nil
	case contains(directionIn, v):
		return model.DirectionIn, nil
	case contains(directionOut, v):
		return model.DirectionOut, nil
	}
	return "", fmt.Errorf("direction %q is not in / out", raw)
}

var lineTypeAliases = map[string][]string{
	model.LineTypePayment: {"payment", "ödeme", "odeme", "tahsilat", "satış", "satis", "sale", "collection", "pos", "kart"},
	model.LineTypeRefund:  {"refund", "iade", "iptal", "chargeback", "ters ibraz", "geri ödeme", "geri odeme"},
	model.LineTypeFee:     {"fee", "komisyon", "commission", "ücret", "ucret", "masraf", "kesinti"},
	model.LineTypePayout:  {"payout", "hakediş", "hakedis", "settlement", "aktarım", "aktarim", "transfer", "eft", "havale", "virman"},
	model.LineTypeOther:   {"other", "diğer", "diger"},
}

// parseLineType maps a sheet value to a line type; an unknown value reads
// as other rather than failing the row.
func parseLineType(raw string) string {
	v := strings.ToLower(strings.TrimSpace(raw))
	if v == "" {
		return ""
	}
	for lineType, aliases := range lineTypeAliases {
		if lineType == v || contains(aliases, v) {
			return lineType
		}
	}
	return model.LineTypeOther
}

func contains(list []string, v string) bool {
	for _, item := range list {
		if item == v {
			return true
		}
	}
	return false
}

var (
	_ ioengine.ResourceAdapter = (*StatementLinesIO)(nil)
	_ ioengine.SampleProvider  = (*StatementLinesIO)(nil)
	_ ioengine.RowPreviewer    = (*StatementLinesIO)(nil)
)
