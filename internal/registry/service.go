// SPDX-License-Identifier: Apache-2.0

package registry

import (
	"context"
	"errors"
	"fmt"
	"os"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"google.golang.org/protobuf/types/known/timestamppb"

	climateshieldv1 "github.com/jarida-io/climateshield/internal/gen/climateshield/v1"
	"github.com/jarida-io/climateshield/internal/gen/climateshield/v1/climateshieldv1connect"
	"github.com/jarida-io/climateshield/internal/platform/config"
	"github.com/jarida-io/climateshield/internal/platform/httpx"
	"github.com/jarida-io/climateshield/internal/platform/logging"
	"github.com/jarida-io/climateshield/internal/platform/metrics"
	"github.com/jarida-io/climateshield/internal/store"
	"github.com/jarida-io/climateshield/internal/store/db"
)

// ServiceConfig configures cmd/registry.
type ServiceConfig struct {
	config.Common
	config.DB
	Addr string `env:"REGISTRY_ADDR" envDefault:":8082"`
}

// Service implements climateshieldv1connect.RegistryServiceHandler.
type Service struct {
	q *db.Queries
}

// NewService builds the registry service over a database handle.
func NewService(dbtx db.DBTX) *Service { return &Service{q: db.New(dbtx)} }

// RecordImmunization appends one immunization event. The event is immutable
// once written (append-only trigger); the ledger service picks it up on its
// next sweep and adds it to the day's Merkle tree.
func (s *Service) RecordImmunization(
	ctx context.Context,
	req *connect.Request[climateshieldv1.RecordImmunizationRequest],
) (*connect.Response[climateshieldv1.RecordImmunizationResponse], error) {
	msg := req.Msg
	if msg.GetChildId() == "" || msg.GetVaccineCode() == "" || msg.GetAdministeredAt() == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			errors.New("child_id, vaccine_code and administered_at are required"))
	}
	var childID pgtype.UUID
	if err := childID.Scan(msg.GetChildId()); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("bad child_id: %w", err))
	}
	if _, err := s.q.GetChild(ctx, childID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("unknown child"))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if err := s.validVaccine(ctx, msg.GetVaccineCode()); err != nil {
		return nil, err
	}

	var facility *string
	if msg.GetFacility() != "" {
		f := msg.GetFacility()
		facility = &f
	}
	row, err := s.q.InsertImmunizationEvent(ctx, db.InsertImmunizationEventParams{
		ChildID:        childID,
		VaccineCode:    msg.GetVaccineCode(),
		AdministeredAt: pgtype.Timestamptz{Time: msg.GetAdministeredAt().AsTime(), Valid: true},
		Facility:       facility,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&climateshieldv1.RecordImmunizationResponse{
		EventId: uuidString(row.ID),
	}), nil
}

// GetDueSummary computes due/overdue vaccines per child (opaque IDs only).
func (s *Service) GetDueSummary(
	ctx context.Context,
	req *connect.Request[climateshieldv1.GetDueSummaryRequest],
) (*connect.Response[climateshieldv1.GetDueSummaryResponse], error) {
	schedule, err := s.Schedule(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	children, err := s.q.ListChildrenForDueComputation(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	pairs, err := s.q.ListImmunizationPairs(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	givenByChild := map[string]map[string]bool{}
	for _, p := range pairs {
		id := uuidString(p.ChildID)
		if givenByChild[id] == nil {
			givenByChild[id] = map[string]bool{}
		}
		givenByChild[id][p.VaccineCode] = true
	}

	resp := &climateshieldv1.GetDueSummaryResponse{}
	now := timestamppb.Now().AsTime()
	for _, c := range children {
		if req.Msg.GetAreaId() != "" && c.AreaID != req.Msg.GetAreaId() {
			continue
		}
		id := uuidString(c.ID)
		for _, d := range DueVaccines(schedule, c.DateOfBirth.Time, givenByChild[id], now) {
			resp.Due = append(resp.Due, &climateshieldv1.DueVaccine{
				ChildId:     id,
				AreaId:      c.AreaID,
				VaccineCode: d.Code,
				DueDate:     d.DueDate.Format("2006-01-02"),
				Overdue:     d.Overdue,
			})
		}
	}
	return connect.NewResponse(resp), nil
}

// Schedule loads the KEPI schedule from the database.
func (s *Service) Schedule(ctx context.Context) ([]ScheduleEntry, error) {
	rows, err := s.q.ListVaccineSchedule(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]ScheduleEntry, 0, len(rows))
	for _, r := range rows {
		out = append(out, ScheduleEntry{
			Code:       r.Code,
			Name:       r.Name,
			DueAgeDays: int(r.DueAgeDays),
			GraceDays:  int(r.OverdueGraceDays),
		})
	}
	return out, nil
}

func (s *Service) validVaccine(ctx context.Context, code string) error {
	schedule, err := s.Schedule(ctx)
	if err != nil {
		return connect.NewError(connect.CodeInternal, err)
	}
	for _, e := range schedule {
		if e.Code == code {
			return nil
		}
	}
	return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("unknown vaccine code %q", code))
}

func uuidString(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	v, err := u.Value()
	if err != nil {
		return ""
	}
	s, _ := v.(string)
	return s
}

// Run starts the registry service (internal Connect API + health/metrics).
func Run(ctx context.Context) error {
	cfg, err := config.Load[ServiceConfig]()
	if err != nil {
		return err
	}
	log := logging.New(os.Stdout, cfg.LogLevel)
	m := metrics.New("registry")

	pool, err := store.Connect(ctx, cfg.URL)
	if err != nil {
		return err
	}
	defer pool.Close()

	router := httpx.NewRouter(func(c context.Context) error { return pool.Ping(c) }, m.Handler())
	path, handler := climateshieldv1connect.NewRegistryServiceHandler(NewService(pool))
	router.Mount(path, handler)

	log.Info("registry started", "addr", cfg.Addr)
	return httpx.Serve(ctx, cfg.Addr, m.Middleware(router), log)
}
