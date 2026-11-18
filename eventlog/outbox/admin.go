package outbox

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/TMS360/backend-pkg/middleware"
	"github.com/TMS360/backend-pkg/response"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Admin actions on a parked event (DEV-3198).
const (
	AdminActionRequeue = "requeue"
	AdminActionDelete  = "delete"
)

// AdminService re-queues or deletes a parked event. Every action needs a
// reason and is logged with who did it; until an admin acts the row stays in
// the table, so nothing is lost.
type AdminService struct {
	repo ParkedRepository
}

func NewAdminService(repo ParkedRepository) *AdminService {
	return &AdminService{repo: repo}
}

// Act applies action ("requeue" or "delete") to the parked event id.
func (s *AdminService) Act(ctx context.Context, action, id, reason string) error {
	if action != AdminActionRequeue && action != AdminActionDelete {
		return response.NewBadRequest("unknown outbox admin action "+action, "Unknown action.")
	}
	if _, err := uuid.Parse(id); err != nil {
		return response.NewBadRequest("invalid outbox event id "+id, "Invalid event id.")
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return response.NewBadRequest("outbox admin action without reason", "A reason is required.")
	}
	actor, err := middleware.GetActor(ctx)
	if err != nil || actor == nil {
		return response.NewUnauthorized("outbox admin action without actor", "Unauthorized.")
	}

	event, err := s.repo.FindParked(ctx, id)
	if err != nil {
		slog.ErrorContext(ctx, "outbox admin: find parked event", "event_id", id, "error", err)
		return response.NewInternalError(err.Error())
	}
	if event == nil {
		return response.NewNotFound("parked outbox event", id)
	}

	var done bool
	if action == AdminActionRequeue {
		done, err = s.repo.Requeue(ctx, id)
	} else {
		done, err = s.repo.DeleteParked(ctx, id)
	}
	if err != nil {
		slog.ErrorContext(ctx, "outbox admin: apply action", "action", action, "event_id", id, "error", err)
		return response.NewInternalError(err.Error())
	}
	if !done {
		// Another admin acted between the read and the write.
		return response.NewNotFound("parked outbox event", id)
	}

	topic := event.Topic
	if topic == "" {
		topic = event.EntityType
	}
	slog.InfoContext(ctx, "outbox admin action on parked event",
		"action", action,
		"event_id", id,
		"topic", topic,
		"entity_id", event.EntityID,
		"size_bytes", len(event.Payload),
		"actor_id", actor.ID,
		"reason", reason)
	return nil
}

// AdminController exposes AdminService over REST. Mount it behind
// RequireAuth + RequireAdmin, e.g. on a service's /internal/admin group:
//
//	outbox.NewAdminController(outbox.NewAdminService(outbox.NewParkedRepository(tm))).Register(internal)
type AdminController struct {
	svc *AdminService
}

func NewAdminController(svc *AdminService) *AdminController {
	return &AdminController{svc: svc}
}

type adminActionRequest struct {
	Reason string `json:"reason"`
}

// Register adds POST /outbox/parked/:id/requeue and POST /outbox/parked/:id/delete.
func (c *AdminController) Register(g gin.IRoutes) {
	g.POST("/outbox/parked/:id/requeue", c.handle(AdminActionRequeue))
	g.POST("/outbox/parked/:id/delete", c.handle(AdminActionDelete))
}

func (c *AdminController) handle(action string) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		var req adminActionRequest
		if err := ctx.ShouldBindJSON(&req); err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "body must be JSON with a reason"})
			return
		}
		id := ctx.Param("id")
		if err := c.svc.Act(ctx.Request.Context(), action, id, req.Reason); err != nil {
			status := http.StatusInternalServerError
			message := "internal error"
			var public response.PublicError
			if errors.As(err, &public) {
				status = public.ErrorStatus()
				message = public.UserMessage()
			}
			ctx.JSON(status, gin.H{"error": message})
			return
		}
		ctx.JSON(http.StatusOK, gin.H{"id": id, "action": action})
	}
}

// Ensure the default repository satisfies both sides.
var (
	_ Repository       = (*repo)(nil)
	_ ParkedRepository = (*repo)(nil)
)
