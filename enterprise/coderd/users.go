package coderd

import (
	"net/http"
	"time"

	"github.com/coder/coder/coderd/audit"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/codersdk"
)

func (api *API) restartRequirementEnabledMW(next http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		// The experiment must be enabled.
		if !api.AGPL.Experiments.Enabled(codersdk.ExperimentTemplateRestartRequirement) {
			httpapi.RouteNotFound(rw)
			return
		}

		// Entitlement must be enabled.
		api.entitlementsMu.RLock()
		entitled := api.entitlements.Features[codersdk.FeatureTemplateRestartRequirement].Entitlement != codersdk.EntitlementNotEntitled
		enabled := api.entitlements.Features[codersdk.FeatureTemplateRestartRequirement].Enabled
		api.entitlementsMu.RUnlock()
		if !entitled {
			httpapi.Write(r.Context(), rw, http.StatusForbidden, codersdk.Response{
				Message: "Template restart requirement is an Enterprise feature. Contact sales!",
			})
			return
		}
		if !enabled {
			httpapi.Write(r.Context(), rw, http.StatusForbidden, codersdk.Response{
				Message: "Template restart requirement feature is not enabled. Please specify a default user quiet hours schedule to use this feature.",
			})
			return
		}

		next.ServeHTTP(rw, r)
	})
}

// @Summary Get user quiet hours schedule
// @ID get-user-quiet-hours-schedule
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param user path string true "User ID" format(uuid)
// @Success 200 {array} codersdk.UserQuietHoursScheduleResponse
// @Router /users/{user}/quiet-hours [get]
func (api *API) userQuietHoursSchedule(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx  = r.Context()
		user = httpmw.UserParam(r)
	)

	opts, err := (*api.UserQuietHoursScheduleStore.Load()).Get(ctx, api.Database, user.ID)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	if opts.Schedule == nil {
		httpapi.ResourceNotFound(rw)
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.UserQuietHoursScheduleResponse{
		RawSchedule: opts.Schedule.String(),
		UserSet:     opts.UserSet,
		Time:        opts.Schedule.Time(),
		Timezone:    opts.Schedule.Location().String(),
		Next:        opts.Schedule.Next(time.Now().In(opts.Schedule.Location())),
	})
}

// @Summary Update user quiet hours schedule
// @ID update-user-quiet-hours-schedule
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Enterprise
// @Param user path string true "User ID" format(uuid)
// @Param request body codersdk.UpdateUserQuietHoursScheduleRequest true "Update schedule request"
// @Success 200 {array} codersdk.UserQuietHoursScheduleResponse
// @Router /users/{user}/quiet-hours [put]
func (api *API) putUserQuietHoursSchedule(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx               = r.Context()
		user              = httpmw.UserParam(r)
		params            codersdk.UpdateUserQuietHoursScheduleRequest
		aReq, commitAudit = audit.InitRequest[database.User](rw, &audit.RequestParams{
			Audit:   api.Auditor,
			Log:     api.Logger,
			Request: r,
			Action:  database.AuditActionWrite,
		})
	)
	defer commitAudit()
	aReq.Old = user

	if !httpapi.Read(ctx, rw, r, &params) {
		return
	}

	opts, err := (*api.UserQuietHoursScheduleStore.Load()).Set(ctx, api.Database, user.ID, params.Schedule)
	if err != nil {
		// TODO(@dean): some of these errors are related to bad syntax, so it
		// would be nice to 400 instead
		httpapi.InternalServerError(rw, err)
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.UserQuietHoursScheduleResponse{
		RawSchedule: opts.Schedule.String(),
		UserSet:     opts.UserSet,
		Time:        opts.Schedule.Time(),
		Timezone:    opts.Schedule.Location().String(),
		Next:        opts.Schedule.Next(time.Now().In(opts.Schedule.Location())),
	})
}
