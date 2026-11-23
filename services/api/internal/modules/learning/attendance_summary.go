package learning

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/school-without-walls/api/internal/platform/response"
)

type attendanceTotals struct {
	Present int64 `json:"present"`
	Absent  int64 `json:"absent"`
	Late    int64 `json:"late"`
	Excused int64 `json:"excused"`
}

type attendanceSummary struct {
	Eligible   int64            `json:"eligible"`
	Unrecorded int64            `json:"unrecorded"`
	OnSite     attendanceTotals `json:"on_site"`
	Remote     attendanceTotals `json:"remote"`
}

func (h *Handler) GetAttendanceSummary(w http.ResponseWriter, r *http.Request) {
	actor, ok := teachingActor(w, r)
	if !ok {
		return
	}
	offering, ok := teachingRouteID(w, r, "offeringID")
	if !ok {
		return
	}
	date, err := time.Parse(time.DateOnly, r.URL.Query().Get("date"))
	if err != nil || date.Year() < 1 {
		invalidAttendance(w, r)
		return
	}

	var summary attendanceSummary
	err = h.pool.QueryRow(r.Context(), `
		WITH eligible AS (SELECT mark.status,mark.mode`+
		attendanceOffering+attendancePlacements+attendanceMarks+attendanceDayEligibility+`)
		SELECT count(*),count(*) FILTER (WHERE mode IS NULL),
			count(*) FILTER (WHERE mode='ON_SITE' AND status='PRESENT'),
			count(*) FILTER (WHERE mode='ON_SITE' AND status='ABSENT'),
			count(*) FILTER (WHERE mode='ON_SITE' AND status='LATE'),
			count(*) FILTER (WHERE mode='ON_SITE' AND status='EXCUSED'),
			count(*) FILTER (WHERE mode='REMOTE' AND status='PRESENT'),
			count(*) FILTER (WHERE mode='REMOTE' AND status='ABSENT'),
			count(*) FILTER (WHERE mode='REMOTE' AND status='LATE'),
			count(*) FILTER (WHERE mode='REMOTE' AND status='EXCUSED')
		FROM eligible HAVING EXISTS (SELECT 1`+attendanceOffering+attendanceOfferingEligibility+` AND $3::uuid IS NULL)`,
		chi.URLParam(r, "slug"), actor, nil, offering, date).
		Scan(&summary.Eligible, &summary.Unrecorded,
			&summary.OnSite.Present, &summary.OnSite.Absent, &summary.OnSite.Late, &summary.OnSite.Excused,
			&summary.Remote.Present, &summary.Remote.Absent, &summary.Remote.Late, &summary.Remote.Excused)
	if err != nil {
		lessonReadError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, summary)
}
