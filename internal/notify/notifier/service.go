// SPDX-License-Identifier: Apache-2.0

package notifier

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"

	"github.com/jarida-io/climateshield/internal/jobs"
	"github.com/jarida-io/climateshield/internal/notify"
	"github.com/jarida-io/climateshield/internal/notify/at"
	"github.com/jarida-io/climateshield/internal/notify/mock"
	notifysmpp "github.com/jarida-io/climateshield/internal/notify/smpp"
	"github.com/jarida-io/climateshield/internal/platform/clock"
	"github.com/jarida-io/climateshield/internal/platform/config"
	"github.com/jarida-io/climateshield/internal/platform/crypto"
	"github.com/jarida-io/climateshield/internal/platform/httpx"
	"github.com/jarida-io/climateshield/internal/platform/logging"
	"github.com/jarida-io/climateshield/internal/platform/metrics"
	"github.com/jarida-io/climateshield/internal/registry"
	"github.com/jarida-io/climateshield/internal/store"
	"github.com/jarida-io/climateshield/internal/store/db"
)

// ServiceConfig configures cmd/notifier.
type ServiceConfig struct {
	config.Common
	config.DB
	Addr           string `env:"NOTIFIER_ADDR" envDefault:":8092"`
	Channel        string `env:"NOTIFY_CHANNEL" envDefault:"mock"`
	MockOutboxPath string `env:"MOCK_OUTBOX_PATH" envDefault:"var/outbox.jsonl"`
	PIIKeyHex      string `env:"PII_KEY_HEX,required"`
	SMPPAddr       string `env:"SMPP_ADDR" envDefault:"localhost:2775"`
	SMPPSystemID   string `env:"SMPP_SYSTEM_ID" envDefault:"climateshield"`
	SMPPPassword   string `env:"SMPP_PASSWORD" envDefault:""`
}

// buildChannel resolves the configured channel adapter.
func buildChannel(cfg ServiceConfig) (notify.Channel, string, error) {
	switch cfg.Channel {
	case mock.ChannelName:
		return mock.New(cfg.MockOutboxPath), mock.ChannelName, nil
	case notifysmpp.ChannelName:
		return notifysmpp.New(cfg.SMPPAddr, cfg.SMPPSystemID, cfg.SMPPPassword), notifysmpp.ChannelName, nil
	case at.ChannelName:
		return nil, "", fmt.Errorf("notify: africastalking: %w", notify.ErrNotConfigured)
	default:
		return nil, "", fmt.Errorf("notify: unknown channel %q", cfg.Channel)
	}
}

// Outcome summarizes one alert_dispatch job.
type Outcome struct {
	WouldSend      int // messages handed to the mock channel
	Sent           int // messages handed to a REAL channel (never mock)
	SkippedConsent int
	Deduplicated   int
}

// dispatchAlert fans one elevated risk score out to guardians of children
// with due vaccines in the affected area. If called during quiet hours it
// performs no work and returns the duration to snooze.
func dispatchAlert(
	ctx context.Context,
	q *db.Queries,
	ch notify.Channel,
	channelName string,
	key crypto.Key,
	clk clock.Clock,
	args jobs.AlertDispatchArgs,
	log *slog.Logger,
) (Outcome, time.Duration, error) {
	var out Outcome
	now := clk.Now()
	if notify.InQuietHours(now) {
		snooze := notify.NextAllowedTime(now).Sub(now)
		log.Info("quiet hours (21:00-07:00 EAT): alert dispatch snoozed",
			"area", args.AreaID, "disease", args.Disease, "resume_in", snooze.Round(time.Minute).String())
		return out, snooze, nil
	}

	schedule, err := loadSchedule(ctx, q)
	if err != nil {
		return out, 0, err
	}
	vaccineNames := map[string]string{}
	for _, e := range schedule {
		vaccineNames[e.Code] = e.Name
	}

	dueByChild, err := dueVaccinesByChild(ctx, q, schedule, args.AreaID, now)
	if err != nil {
		return out, 0, err
	}

	areaName, err := areaDisplayName(ctx, q, args.AreaID)
	if err != nil {
		return out, 0, err
	}

	for _, cd := range dueByChild {
		// One alert per child per risk score, ever.
		exists, err := q.ExistsAlertForChildRisk(ctx, db.ExistsAlertForChildRiskParams{
			ChildID: cd.childID, RiskScoreID: &args.RiskScoreID,
		})
		if err != nil {
			return out, 0, fmt.Errorf("notify: dedup check: %w", err)
		}
		if exists {
			out.Deduplicated++
			continue
		}

		child, err := q.GetChild(ctx, cd.childID)
		if err != nil {
			return out, 0, fmt.Errorf("notify: load child: %w", err)
		}
		guardian, err := q.GetGuardian(ctx, child.GuardianID)
		if err != nil {
			return out, 0, fmt.Errorf("notify: load guardian: %w", err)
		}

		alert := db.InsertAlertParams{
			RiskScoreID: &args.RiskScoreID,
			ChildID:     child.ID,
			GuardianID:  guardian.ID,
			AreaID:      args.AreaID,
			VaccineCode: cd.vaccineCode,
			Lang:        guardian.Lang,
			Channel:     channelName,
		}

		// Consent gate: the latest consent_log row must be OPT_IN.
		consent, err := q.LatestConsent(ctx, guardian.ID)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return out, 0, fmt.Errorf("notify: consent: %w", err)
		}
		if consent != "OPT_IN" {
			alert.Status = "skipped_consent"
			if _, err := q.InsertAlert(ctx, alert); err != nil {
				return out, 0, fmt.Errorf("notify: record skip: %w", err)
			}
			out.SkippedConsent++
			continue
		}

		childName, err := crypto.FromBytes[string](child.NameEnc).Open(key)
		if err != nil {
			return out, 0, fmt.Errorf("notify: decrypt child name: %w", err)
		}
		phone, err := crypto.FromBytes[string](guardian.PhoneEnc).Open(key)
		if err != nil {
			return out, 0, fmt.Errorf("notify: decrypt guardian phone: %w", err)
		}

		body, err := notify.Render(notify.TemplateData{
			Lang:        guardian.Lang,
			RiskLevel:   args.Level,
			County:      areaName,
			FirstName:   firstName(childName),
			VaccineName: vaccineNames[cd.vaccineCode],
		})
		if err != nil {
			return out, 0, err
		}

		msgID, err := ch.Send(ctx, notify.Recipient{Phone: phone, Lang: guardian.Lang}, notify.Message{Body: body})
		if err != nil {
			return out, 0, fmt.Errorf("notify: send via %s: %w", channelName, err)
		}

		// Honesty is encoded in the status: the mock channel records
		// would_send; only a real carrier adapter records sent.
		if channelName == mock.ChannelName {
			alert.Status = "would_send"
		} else {
			alert.Status = "sent"
		}
		hash := sha256.Sum256([]byte(body))
		alert.MessageHash = hash[:]
		id := string(msgID)
		alert.MessageID = &id
		alert.DispatchedAt = pgtype.Timestamptz{Time: now.UTC(), Valid: true}
		if _, err := q.InsertAlert(ctx, alert); err != nil {
			return out, 0, fmt.Errorf("notify: record alert: %w", err)
		}
		if channelName == mock.ChannelName {
			out.WouldSend++
		} else {
			out.Sent++
		}
	}

	logOutcome(log, channelName, args, out)
	return out, 0, nil
}

// logOutcome states exactly what happened — and, for the mock channel, what
// deliberately did NOT happen.
func logOutcome(log *slog.Logger, channelName string, args jobs.AlertDispatchArgs, out Outcome) {
	if channelName == mock.ChannelName {
		log.Info(fmt.Sprintf("[mock] would send %d alerts", out.WouldSend),
			"area", args.AreaID, "disease", args.Disease, "level", args.Level,
			"skipped_consent", out.SkippedConsent, "deduplicated", out.Deduplicated,
			"note", "mock channel active: no SMS was sent")
		return
	}
	log.Info(fmt.Sprintf("sent %d alerts via %s", out.Sent, channelName),
		"area", args.AreaID, "disease", args.Disease, "level", args.Level,
		"skipped_consent", out.SkippedConsent, "deduplicated", out.Deduplicated)
}

type childDue struct {
	childID     pgtype.UUID
	vaccineCode string
}

// dueVaccinesByChild returns one entry per child in the area with at least
// one due vaccine — the earliest-due one, which is what the SMS mentions.
func dueVaccinesByChild(
	ctx context.Context,
	q *db.Queries,
	schedule []registry.ScheduleEntry,
	areaID string,
	now time.Time,
) ([]childDue, error) {
	children, err := q.ListChildrenForDueComputation(ctx)
	if err != nil {
		return nil, fmt.Errorf("notify: list children: %w", err)
	}
	pairs, err := q.ListImmunizationPairs(ctx)
	if err != nil {
		return nil, fmt.Errorf("notify: list events: %w", err)
	}
	given := map[pgtype.UUID]map[string]bool{}
	for _, p := range pairs {
		if given[p.ChildID] == nil {
			given[p.ChildID] = map[string]bool{}
		}
		given[p.ChildID][p.VaccineCode] = true
	}

	var out []childDue
	for _, c := range children {
		if c.AreaID != areaID {
			continue
		}
		due := registry.DueVaccines(schedule, c.DateOfBirth.Time, given[c.ID], now)
		if len(due) == 0 {
			continue
		}
		sort.Slice(due, func(i, j int) bool { return due[i].DueDate.Before(due[j].DueDate) })
		out = append(out, childDue{childID: c.ID, vaccineCode: due[0].Code})
	}
	return out, nil
}

func loadSchedule(ctx context.Context, q *db.Queries) ([]registry.ScheduleEntry, error) {
	rows, err := q.ListVaccineSchedule(ctx)
	if err != nil {
		return nil, fmt.Errorf("notify: schedule: %w", err)
	}
	out := make([]registry.ScheduleEntry, 0, len(rows))
	for _, r := range rows {
		out = append(out, registry.ScheduleEntry{
			Code: r.Code, Name: r.Name,
			DueAgeDays: int(r.DueAgeDays), GraceDays: int(r.OverdueGraceDays),
		})
	}
	return out, nil
}

func areaDisplayName(ctx context.Context, q *db.Queries, areaID string) (string, error) {
	areas, err := q.ListAreas(ctx)
	if err != nil {
		return "", fmt.Errorf("notify: areas: %w", err)
	}
	for _, a := range areas {
		if a.ID == areaID {
			return a.Name, nil
		}
	}
	return "", fmt.Errorf("notify: unknown area %q", areaID)
}

func firstName(full string) string {
	if i := strings.IndexByte(full, ' '); i > 0 {
		return full[:i]
	}
	return full
}

// dispatchWorker is the River worker for alert_dispatch jobs.
type dispatchWorker struct {
	river.WorkerDefaults[jobs.AlertDispatchArgs]
	pool interface {
		db.DBTX
	}
	ch          notify.Channel
	channelName string
	key         crypto.Key
	clk         clock.Clock
	log         *slog.Logger
}

// Work implements river.Worker.
func (w *dispatchWorker) Work(ctx context.Context, job *river.Job[jobs.AlertDispatchArgs]) error {
	_, snooze, err := dispatchAlert(ctx, db.New(w.pool), w.ch, w.channelName, w.key, w.clk, job.Args, w.log)
	if err != nil {
		return err
	}
	if snooze > 0 {
		return river.JobSnooze(snooze)
	}
	return nil
}

// Run starts the notifier service.
func Run(ctx context.Context) error {
	cfg, err := config.Load[ServiceConfig]()
	if err != nil {
		return err
	}
	log := logging.New(os.Stdout, cfg.LogLevel)
	m := metrics.New("notifier")

	key, err := crypto.KeyFromHex(cfg.PIIKeyHex)
	if err != nil {
		return err
	}
	ch, channelName, err := buildChannel(cfg)
	if err != nil {
		return err
	}

	pool, err := store.Connect(ctx, cfg.URL)
	if err != nil {
		return err
	}
	defer pool.Close()

	workers := river.NewWorkers()
	river.AddWorker(workers, &dispatchWorker{
		pool: pool, ch: ch, channelName: channelName, key: key, clk: clock.Real{}, log: log,
	})

	riverClient, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Queues:  map[string]river.QueueConfig{jobs.QueueNotify: {MaxWorkers: 2}},
		Workers: workers,
	})
	if err != nil {
		return err
	}
	if err := riverClient.Start(ctx); err != nil {
		return err
	}
	defer func() { _ = riverClient.Stop(context.Background()) }()

	log.Info("notifier started", "channel", channelName,
		"note", map[bool]string{true: "mock channel: alerts are recorded, no SMS is sent", false: "live channel"}[channelName == mock.ChannelName])
	return httpx.Serve(ctx, cfg.Addr, httpx.NewRouter(func(c context.Context) error {
		return pool.Ping(c)
	}, m.Handler()), log)
}
