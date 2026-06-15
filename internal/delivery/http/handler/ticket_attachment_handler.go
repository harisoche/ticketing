package handler

import (
	"errors"
	"io"
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"ticketing-api/internal/delivery/http/middleware"
	"ticketing-api/internal/delivery/http/response"
	"ticketing-api/internal/usecase/ticket"
)

type TicketAttachmentHandler struct {
	svc *ticket.Service
	max int64
}

func NewTicketAttachmentHandler(svc *ticket.Service, maxBytes int64) *TicketAttachmentHandler {
	return &TicketAttachmentHandler{svc: svc, max: maxBytes}
}

func (h *TicketAttachmentHandler) Upload(c echo.Context) error {
	ticketID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return response.JSONError(c, http.StatusUnprocessableEntity, "invalid ticket id")
	}
	ctx := c.Request().Context()
	actor, err := h.svc.Actor(ctx, middleware.AuthenticatedUserID(c))
	if err != nil {
		return translateErr(c, err)
	}

	// Cap incoming request body up front. Use max+1 so the use case can
	// distinguish "right at the limit" from "exceeds limit".
	if h.max > 0 {
		c.Request().Body = http.MaxBytesReader(c.Response().Writer, c.Request().Body, h.max+1)
	}

	file, err := c.FormFile("file")
	if err != nil {
		if errors.Is(err, http.ErrMissingFile) {
			return response.JSONError(c, http.StatusUnprocessableEntity, "file is required")
		}
		// MaxBytesReader returns a generic error when the body is too large;
		// the use case will catch the size cap as well, but report early.
		return response.JSONError(c, http.StatusRequestEntityTooLarge, "uploaded file is too large")
	}
	src, err := file.Open()
	if err != nil {
		return response.JSONError(c, http.StatusInternalServerError, "could not read uploaded file")
	}
	defer src.Close()

	out, err := h.svc.AddAttachment(ctx, actor, ticketID, src, file.Filename, file.Size)
	if err != nil {
		return translateErr(c, err)
	}
	return response.Created(c, out, "attachment uploaded successfully")
}

func (h *TicketAttachmentHandler) List(c echo.Context) error {
	ticketID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return response.JSONError(c, http.StatusUnprocessableEntity, "invalid ticket id")
	}
	ctx := c.Request().Context()
	actor, err := h.svc.Actor(ctx, middleware.AuthenticatedUserID(c))
	if err != nil {
		return translateErr(c, err)
	}
	out, err := h.svc.ListAttachments(ctx, actor, ticketID)
	if err != nil {
		return translateErr(c, err)
	}
	return response.OK(c, out, "attachments retrieved successfully")
}

func (h *TicketAttachmentHandler) Download(c echo.Context) error {
	ticketID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return response.JSONError(c, http.StatusUnprocessableEntity, "invalid ticket id")
	}
	attachmentID, err := uuid.Parse(c.Param("attachment_id"))
	if err != nil {
		return response.JSONError(c, http.StatusUnprocessableEntity, "invalid attachment id")
	}
	ctx := c.Request().Context()
	actor, err := h.svc.Actor(ctx, middleware.AuthenticatedUserID(c))
	if err != nil {
		return translateErr(c, err)
	}
	meta, rc, err := h.svc.DownloadAttachment(ctx, actor, ticketID, attachmentID)
	if err != nil {
		return translateErr(c, err)
	}
	defer rc.Close()

	res := c.Response()
	res.Header().Set(echo.HeaderContentType, meta.MimeType)
	res.Header().Set(echo.HeaderContentDisposition, `attachment; filename="`+meta.OriginalFilename+`"`)
	res.WriteHeader(http.StatusOK)
	if _, err := io.Copy(res.Writer, rc); err != nil {
		return err
	}
	return nil
}

func (h *TicketAttachmentHandler) Delete(c echo.Context) error {
	ticketID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return response.JSONError(c, http.StatusUnprocessableEntity, "invalid ticket id")
	}
	attachmentID, err := uuid.Parse(c.Param("attachment_id"))
	if err != nil {
		return response.JSONError(c, http.StatusUnprocessableEntity, "invalid attachment id")
	}
	ctx := c.Request().Context()
	actor, err := h.svc.Actor(ctx, middleware.AuthenticatedUserID(c))
	if err != nil {
		return translateErr(c, err)
	}
	if err := h.svc.DeleteAttachment(ctx, actor, ticketID, attachmentID); err != nil {
		return translateErr(c, err)
	}
	return response.NoContent(c)
}
