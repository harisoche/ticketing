// cmd/seed inserts development fixtures into the local DB. It is idempotent
// — re-running it does not duplicate users or tickets.
//
// Usage:
//
//	go run ./cmd/seed
//
// WARNING: the default password lives in code/env, not in your password
// manager. Never run this against production.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"ticketing-api/internal/config"
	"ticketing-api/internal/infrastructure/database"
	"ticketing-api/internal/infrastructure/persistence/model"
)

type seedUser struct {
	Name  string
	Email string
	Role  string
}

var seedUsers = []seedUser{
	{Name: "Demo Admin", Email: "admin@example.com", Role: "admin"},
	{Name: "Demo Agent One", Email: "agent1@example.com", Role: "agent"},
	{Name: "Demo Agent Two", Email: "agent2@example.com", Role: "agent"},
	{Name: "Demo Customer One", Email: "user1@example.com", Role: "customer"},
	{Name: "Demo Customer Two", Email: "user2@example.com", Role: "customer"},
}

type seedTicket struct {
	Title       string
	Description string
	Priority    string
	Status      string
	Creator     string // email
	Assignee    string // email or ""
	Category    string // slug
}

var seedTickets = []seedTicket{
	{
		Title:       "Cannot log in from mobile app",
		Description: "After updating to v3.1 the customer app rejects valid credentials.",
		Priority:    "high",
		Status:      "in_progress",
		Creator:     "user1@example.com",
		Assignee:    "agent1@example.com",
		Category:    "technical-issue",
	},
	{
		Title:       "Wrong invoice total in May statement",
		Description: "The May 2026 invoice double-counted the storage line item.",
		Priority:    "medium",
		Status:      "open",
		Creator:     "user1@example.com",
		Category:    "billing-issue",
	},
	{
		Title:       "Need to update primary email address",
		Description: "Please change my account email to the new domain.",
		Priority:    "low",
		Status:      "resolved",
		Creator:     "user2@example.com",
		Assignee:    "agent2@example.com",
		Category:    "account-issue",
	},
	{
		Title:       "Urgent: payment is stuck in pending",
		Description: "Customer reported a payment in pending state for 24 hours.",
		Priority:    "urgent",
		Status:      "open",
		Creator:     "user2@example.com",
		Category:    "billing-issue",
	},
}

func main() {
	if err := run(); err != nil {
		log.Fatalf("seed failed: %v", err)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	db, err := database.Open(database.DefaultOptions(cfg.DatabaseURL))
	if err != nil {
		return err
	}
	defer database.Close(db) //nolint:errcheck

	password := os.Getenv("SEED_ADMIN_PASSWORD")
	if password == "" {
		password = "password123"
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("bcrypt: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_ = ctx

	emailToID := map[string]int64{}
	for _, u := range seedUsers {
		var row model.UserModel
		err := db.Where("LOWER(email) = ?", strings.ToLower(u.Email)).First(&row).Error
		if err == nil {
			// already exists — keep current password, just align role.
			if row.Role != u.Role {
				if err := db.Model(&row).Update("role", u.Role).Error; err != nil {
					return err
				}
			}
			emailToID[u.Email] = row.ID
			continue
		}
		m := &model.UserModel{
			Name:         u.Name,
			Email:        strings.ToLower(u.Email),
			PasswordHash: string(hash),
			Role:         u.Role,
		}
		if err := db.Create(m).Error; err != nil {
			return fmt.Errorf("create %s: %w", u.Email, err)
		}
		emailToID[u.Email] = m.ID
	}

	// Load category slug → uuid.
	var cats []model.TicketCategoryModel
	if err := db.Find(&cats).Error; err != nil {
		return err
	}
	slugToCategory := map[string]model.TicketCategoryModel{}
	for _, c := range cats {
		slugToCategory[c.Slug] = c
	}

	// Demo tickets. Idempotent by title — re-run won't duplicate.
	for _, t := range seedTickets {
		creatorID, ok := emailToID[t.Creator]
		if !ok {
			return fmt.Errorf("seed user not found: %s", t.Creator)
		}
		category, ok := slugToCategory[t.Category]
		if !ok {
			return fmt.Errorf("seed category not found: %s", t.Category)
		}
		var existing model.TicketModel
		if err := db.Where("title = ? AND created_by = ?", t.Title, creatorID).First(&existing).Error; err == nil {
			continue
		}
		tk := &model.TicketModel{
			ID:          uuid.New(),
			Title:       t.Title,
			Description: t.Description,
			Status:      t.Status,
			Priority:    t.Priority,
			CategoryID:  category.ID,
			CreatedBy:   creatorID,
		}
		if t.Assignee != "" {
			id, ok := emailToID[t.Assignee]
			if !ok {
				return fmt.Errorf("seed assignee not found: %s", t.Assignee)
			}
			tk.AssignedTo = &id
			now := time.Now().UTC()
			tk.AssignedAt = &now
		}
		// SLA due times are recomputed by the API; for seed simplicity we
		// leave them null. They'll be filled in when an agent next touches
		// the ticket (since priority changes / status changes recompute).
		if err := db.Create(tk).Error; err != nil {
			return fmt.Errorf("create ticket %q: %w", t.Title, err)
		}
	}

	fmt.Println("seed complete.")
	fmt.Println()
	fmt.Println("Development accounts (password: " + password + "):")
	for _, u := range seedUsers {
		fmt.Printf("  %-10s %s\n", u.Role, u.Email)
	}
	fmt.Println()
	fmt.Println("DO NOT use these credentials in production.")
	return nil
}
