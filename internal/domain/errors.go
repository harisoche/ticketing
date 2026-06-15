package domain

import "errors"

// Sentinel domain errors. Adapters (HTTP, repositories, services) translate
// these to user-facing responses or library-specific errors.
var (
	ErrUserNotFound       = errors.New("user not found")
	ErrEmailAlreadyExists = errors.New("email already exists")
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrSessionNotFound    = errors.New("session not found")
	ErrUnauthorized       = errors.New("unauthorized")
	ErrForbidden          = errors.New("forbidden")
	ErrInvalidInput       = errors.New("invalid input")

	ErrTicketNotFound          = errors.New("ticket not found")
	ErrTicketCategoryNotFound  = errors.New("ticket category not found")
	ErrInvalidStatusTransition = errors.New("invalid ticket status transition")
	ErrInvalidAssignee         = errors.New("invalid ticket assignee")
	ErrTicketNotAssigned       = errors.New("ticket must be assigned to an agent before it can progress")
	ErrCommentNotFound         = errors.New("ticket comment not found")
	ErrAttachmentNotFound      = errors.New("ticket attachment not found")
	ErrAttachmentTooLarge      = errors.New("attachment exceeds maximum allowed size")
	ErrAttachmentUnsupported   = errors.New("attachment mime type is not supported")
	ErrCategoryConflict        = errors.New("category name or slug already in use")
	ErrSLAPolicyNotFound       = errors.New("sla policy not found")
	ErrNotificationNotFound    = errors.New("notification not found")
)
