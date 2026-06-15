package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"ticketing-api/internal/domain"
	"ticketing-api/internal/domain/entity"
	"ticketing-api/internal/domain/repository"
	"ticketing-api/internal/infrastructure/persistence/model"
)

type notificationRepository struct {
	db *gorm.DB
}

func NewNotificationRepository(db *gorm.DB) repository.NotificationRepository {
	return &notificationRepository{db: db}
}

func (r *notificationRepository) CreateMany(ctx context.Context, notifications []entity.Notification) error {
	if len(notifications) == 0 {
		return nil
	}
	rows := make([]model.NotificationModel, 0, len(notifications))
	for i := range notifications {
		if notifications[i].ID == uuid.Nil {
			notifications[i].ID = uuid.New()
		}
		rows = append(rows, *model.NotificationModelFromEntity(&notifications[i]))
	}
	if err := dbFrom(ctx, r.db).Create(&rows).Error; err != nil {
		return err
	}
	for i := range rows {
		notifications[i].CreatedAt = rows[i].CreatedAt
	}
	return nil
}

func (r *notificationRepository) ListByRecipient(ctx context.Context, recipientID int64, filter repository.NotificationListFilter) ([]entity.Notification, int64, int64, error) {
	page := filter.Page
	if page < 1 {
		page = 1
	}
	per := filter.PerPage
	if per < 1 {
		per = 20
	}
	if per > 100 {
		per = 100
	}

	base := func() *gorm.DB {
		q := dbFrom(ctx, r.db).Model(&model.NotificationModel{}).Where("recipient_id = ?", recipientID)
		if filter.UnreadOnly {
			q = q.Where("read_at IS NULL")
		}
		return q
	}

	var total int64
	if err := base().Count(&total).Error; err != nil {
		return nil, 0, 0, err
	}
	var unread int64
	if err := dbFrom(ctx, r.db).
		Model(&model.NotificationModel{}).
		Where("recipient_id = ? AND read_at IS NULL", recipientID).
		Count(&unread).Error; err != nil {
		return nil, 0, 0, err
	}

	var rows []model.NotificationModel
	if err := base().
		Order("created_at DESC, id DESC").
		Limit(per).
		Offset((page - 1) * per).
		Find(&rows).Error; err != nil {
		return nil, 0, 0, err
	}
	out := make([]entity.Notification, 0, len(rows))
	for i := range rows {
		out = append(out, *rows[i].ToEntity())
	}
	return out, total, unread, nil
}

func (r *notificationRepository) FindByID(ctx context.Context, id uuid.UUID) (*entity.Notification, error) {
	var m model.NotificationModel
	if err := dbFrom(ctx, r.db).First(&m, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotificationNotFound
		}
		return nil, err
	}
	return m.ToEntity(), nil
}

func (r *notificationRepository) MarkRead(ctx context.Context, recipientID int64, id uuid.UUID, readAt time.Time) error {
	// Look up first so we can distinguish "not found / not yours" from
	// "already read".
	var m model.NotificationModel
	if err := dbFrom(ctx, r.db).
		Where("id = ? AND recipient_id = ?", id, recipientID).
		First(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.ErrNotificationNotFound
		}
		return err
	}
	if m.ReadAt != nil {
		return nil // idempotent
	}
	return dbFrom(ctx, r.db).
		Model(&model.NotificationModel{}).
		Where("id = ? AND recipient_id = ? AND read_at IS NULL", id, recipientID).
		Update("read_at", readAt).Error
}

func (r *notificationRepository) MarkAllRead(ctx context.Context, recipientID int64, readAt time.Time) (int64, error) {
	res := dbFrom(ctx, r.db).
		Model(&model.NotificationModel{}).
		Where("recipient_id = ? AND read_at IS NULL", recipientID).
		Update("read_at", readAt)
	if res.Error != nil {
		return 0, res.Error
	}
	return res.RowsAffected, nil
}

func (r *notificationRepository) UnreadCount(ctx context.Context, recipientID int64) (int64, error) {
	var count int64
	err := dbFrom(ctx, r.db).
		Model(&model.NotificationModel{}).
		Where("recipient_id = ? AND read_at IS NULL", recipientID).
		Count(&count).Error
	return count, err
}
