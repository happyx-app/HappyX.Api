package usecase

import (
	"context"
	"time"

	"github.com/happyx-app/HappyX.Api/internal/domain"
)

// AdminRepository reads the counts behind the owner's admin dashboard: every guild, but never a
// mood, a message or a name a member wrote.
type AdminRepository interface {
	Overview(ctx context.Context, now time.Time) (domain.AdminOverview, error)
	Guilds(ctx context.Context, now time.Time) ([]domain.AdminGuild, error)
	Guild(ctx context.Context, guildID string, now time.Time) (domain.AdminGuildDetail, error)
	// Deliveries returns the last n days of sends and failed DMs, oldest first.
	Deliveries(ctx context.Context, now time.Time, n int) ([]domain.AdminDeliveryDay, error)
	Database(ctx context.Context) (domain.AdminDatabase, error)
	Settings(ctx context.Context) (domain.AdminSettings, error)
}

// BackupStore lists the database backups, newest first. ok is false when no backup folder is
// configured.
type BackupStore interface {
	List(ctx context.Context) (backups []domain.Backup, ok bool, err error)
}

// Admin serves the owner's admin dashboard. It only reads.
type Admin struct {
	repo    AdminRepository
	backups BackupStore
	now     Clock
}

func NewAdmin(repo AdminRepository, backups BackupStore, now Clock) *Admin {
	if now == nil {
		now = time.Now
	}
	return &Admin{repo: repo, backups: backups, now: now}
}

// DeliveryDays is how many days of sends the health page shows.
const DeliveryDays = 14

func (a *Admin) Now() time.Time { return a.now() }

func (a *Admin) Overview(ctx context.Context) (domain.AdminOverview, error) {
	return a.repo.Overview(ctx, a.now())
}

func (a *Admin) Guilds(ctx context.Context) ([]domain.AdminGuild, error) {
	return a.repo.Guilds(ctx, a.now())
}

func (a *Admin) Guild(ctx context.Context, guildID string) (domain.AdminGuildDetail, error) {
	return a.repo.Guild(ctx, guildID, a.now())
}

func (a *Admin) Deliveries(ctx context.Context) ([]domain.AdminDeliveryDay, error) {
	return a.repo.Deliveries(ctx, a.now(), DeliveryDays)
}

func (a *Admin) Database(ctx context.Context) (domain.AdminDatabase, error) {
	return a.repo.Database(ctx)
}

func (a *Admin) Settings(ctx context.Context) (domain.AdminSettings, error) {
	return a.repo.Settings(ctx)
}

func (a *Admin) Backups(ctx context.Context) ([]domain.Backup, bool, error) {
	if a.backups == nil {
		return nil, false, nil
	}
	return a.backups.List(ctx)
}
