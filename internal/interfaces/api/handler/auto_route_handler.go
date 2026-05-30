package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"
	appRouting "github.com/trailyai/traffic-ai/internal/application/routing"
	domainRouting "github.com/trailyai/traffic-ai/internal/domain/routing"
	"github.com/trailyai/traffic-ai/internal/interfaces/api/dto"
	"github.com/trailyai/traffic-ai/pkg/errcode"
	"github.com/trailyai/traffic-ai/pkg/response"
)

type AutoRouteHandler struct {
	routingUC *appRouting.UseCase
}

func NewAutoRouteHandler(routingUC *appRouting.UseCase) *AutoRouteHandler {
	return &AutoRouteHandler{routingUC: routingUC}
}

func (h *AutoRouteHandler) Register(group *gin.RouterGroup) {
	group.GET("/auto-routes", h.ListPolicies)
	group.POST("/auto-routes", h.CreatePolicy)
	group.PATCH("/auto-routes/:id", h.UpdatePolicy)
	group.DELETE("/auto-routes/:id", h.DeletePolicy)
	group.GET("/auto-routes/:id/candidates", h.ListCandidates)
	group.POST("/auto-routes/:id/candidates", h.CreateCandidate)
	group.PATCH("/auto-routes/:id/candidates/:candidateId", h.UpdateCandidate)
	group.DELETE("/auto-routes/:id/candidates/:candidateId", h.DeleteCandidate)
	group.POST("/auto-routes/:id/test", h.DryRun)
}

func (h *AutoRouteHandler) ListPolicies(c *gin.Context) {
	list, err := h.routingUC.ListAutoRoutePolicies(c.Request.Context())
	if err != nil {
		handleError(c, err)
		return
	}
	response.OK(c, gin.H{"items": dto.ToAutoRoutePolicyItems(list)})
}

func (h *AutoRouteHandler) CreatePolicy(c *gin.Context) {
	var req dto.AutoRoutePolicyReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ErrBadRequest)
		return
	}
	p := req.ToDomain(0)
	if err := h.routingUC.CreateAutoRoutePolicy(c.Request.Context(), p); err != nil {
		handleError(c, err)
		return
	}
	response.OK(c, dto.ToAutoRoutePolicyItem(p))
}

func (h *AutoRouteHandler) UpdatePolicy(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	var req dto.AutoRoutePolicyReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ErrBadRequest)
		return
	}
	p := req.ToDomain(id)
	if err := h.routingUC.UpdateAutoRoutePolicy(c.Request.Context(), p); err != nil {
		handleError(c, err)
		return
	}
	response.OK(c, dto.ToAutoRoutePolicyItem(p))
}

func (h *AutoRouteHandler) DeletePolicy(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	if err := h.routingUC.DeleteAutoRoutePolicy(c.Request.Context(), id); err != nil {
		handleError(c, err)
		return
	}
	response.OK(c, gin.H{"deleted": true})
}

func (h *AutoRouteHandler) ListCandidates(c *gin.Context) {
	policyID, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	list, err := h.routingUC.ListAutoRouteCandidates(c.Request.Context(), policyID, false)
	if err != nil {
		handleError(c, err)
		return
	}
	response.OK(c, gin.H{"items": dto.ToAutoRouteCandidateItems(list)})
}

func (h *AutoRouteHandler) CreateCandidate(c *gin.Context) {
	policyID, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	var req dto.AutoRouteCandidateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ErrBadRequest)
		return
	}
	candidate := req.ToDomain(0, policyID)
	if err := h.routingUC.CreateAutoRouteCandidate(c.Request.Context(), candidate); err != nil {
		handleError(c, err)
		return
	}
	response.OK(c, dto.ToAutoRouteCandidateItem(candidate))
}

func (h *AutoRouteHandler) UpdateCandidate(c *gin.Context) {
	policyID, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	candidateID, ok := parseIDParam(c, "candidateId")
	if !ok {
		return
	}
	var req dto.AutoRouteCandidateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ErrBadRequest)
		return
	}
	candidate := req.ToDomain(candidateID, policyID)
	if err := h.routingUC.UpdateAutoRouteCandidate(c.Request.Context(), candidate); err != nil {
		handleError(c, err)
		return
	}
	response.OK(c, dto.ToAutoRouteCandidateItem(candidate))
}

func (h *AutoRouteHandler) DeleteCandidate(c *gin.Context) {
	candidateID, ok := parseIDParam(c, "candidateId")
	if !ok {
		return
	}
	if err := h.routingUC.DeleteAutoRouteCandidate(c.Request.Context(), candidateID); err != nil {
		handleError(c, err)
		return
	}
	response.OK(c, gin.H{"deleted": true})
}

func (h *AutoRouteHandler) DryRun(c *gin.Context) {
	var req dto.AutoRouteDryRunReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ErrBadRequest)
		return
	}
	if req.TokenGroup == "" {
		req.TokenGroup = "default"
	}
	if req.Protocol == "" {
		req.Protocol = "openai"
	}
	route, err := h.routingUC.SelectRoute(c.Request.Context(), domainRouting.RouteRequest{
		TokenGroup:      req.TokenGroup,
		RequestedModel:  req.RequestedModel,
		Protocol:        req.Protocol,
		EstimatedTokens: req.EstimatedTokens,
		Stream:          req.Stream,
	})
	if err != nil {
		handleError(c, err)
		return
	}
	response.OK(c, gin.H{
		"requested_model":  route.RequestedModel,
		"resolved_model":   route.ResolvedModel,
		"model_account_id": route.Account.ID,
		"policy_id":        route.PolicyID,
		"mode":             route.Mode,
		"score":            route.Score,
		"reason":           route.Reason,
	})
}

func parseIDParam(c *gin.Context, name string) (int64, bool) {
	raw := c.Param(name)
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		response.Fail(c, errcode.ErrBadRequest)
		return 0, false
	}
	return id, true
}
