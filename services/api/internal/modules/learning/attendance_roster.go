package learning

import (
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type attendanceRosterItem struct {
	UpdatedAt   *time.Time `json:"updated_at,omitempty"`
	ID          uuid.UUID  `json:"id"`
	DisplayName string     `json:"display_name"`
	Status      *string    `json:"status,omitempty"`
	Mode        *string    `json:"mode,omitempty"`
}

const attendanceOffering = teacherOfferingJoins + `
	JOIN academic_terms term ON term.tenant_id=o.tenant_id AND term.id=o.term_id
	JOIN academic_sessions sess ON sess.tenant_id=term.tenant_id AND sess.id=term.session_id AND sess.status='ACTIVE'
	JOIN classes c ON c.tenant_id=o.tenant_id AND c.session_id=sess.id AND c.id=o.class_id AND c.status='ACTIVE'
	JOIN grade_levels g ON g.tenant_id=c.tenant_id AND g.id=c.grade_level_id AND g.status='ACTIVE'
	LEFT JOIN class_arms offered_arm ON offered_arm.tenant_id=o.tenant_id AND offered_arm.id=o.class_arm_id
	CROSS JOIN LATERAL (SELECT (statement_timestamp() AT TIME ZONE t.timezone)::date AS local_today,
		(($5::date+1)::timestamp AT TIME ZONE t.timezone) AS next_midnight,
		($5::date::timestamp AT TIME ZONE t.timezone) AS day_start) day
`

// Select the most recent placement before the end of a historical day.
// Use the effective interval at statement time for the current day.
const attendancePlacements = `
	JOIN LATERAL (
		SELECT DISTINCT ON (e.student_id) e.*
		FROM enrollments e WHERE e.tenant_id=t.id AND e.term_id=term.id
			AND (($5::date=day.local_today AND e.created_at<=statement_timestamp()
				AND (e.ended_at IS NULL OR e.ended_at>statement_timestamp()))
			OR ($5::date<day.local_today AND e.created_at<day.next_midnight
				AND (e.ended_at IS NULL OR e.ended_at>day.day_start)))
		ORDER BY e.student_id,e.created_at DESC,e.id
	) placement ON placement.class_id=c.id AND (o.class_arm_id IS NULL OR o.class_arm_id=placement.class_arm_id)
	JOIN students student ON student.tenant_id=t.id AND student.id=placement.student_id AND student.status='ACTIVE'
	LEFT JOIN class_arms placement_arm ON placement_arm.tenant_id=t.id AND placement_arm.id=placement.class_arm_id
`

const attendanceOfferingEligibility = `
	WHERE t.slug=$1::citext AND t.status='ACTIVE' AND o.id=$4 AND $5::date<=day.local_today
	AND $5::date BETWEEN term.starts_on AND term.ends_on
	AND (o.class_arm_id IS NULL OR offered_arm.status='ACTIVE')
`

const attendanceDayEligibility = attendanceOfferingEligibility + `
	AND (placement.class_arm_id IS NULL OR placement_arm.status='ACTIVE')
`

const attendanceMarks = `
	LEFT JOIN attendance_records mark ON mark.tenant_id=t.id AND mark.student_id=student.id
		AND mark.session_id=sess.id AND mark.class_id=c.id AND mark.on_date=$5::date
`

func (h *Handler) ListAttendanceRoster(w http.ResponseWriter, r *http.Request) {
	offering, ok := teachingRouteID(w, r, "offeringID")
	if !ok {
		return
	}
	date, err := time.Parse(time.DateOnly, r.URL.Query().Get("date"))
	if err != nil || date.Year() < 1 {
		invalidLesson(w, r)
		return
	}
	teacherPage(h, w, r, pgx.RowToStructByName[attendanceRosterItem], func(row attendanceRosterItem) uuid.UUID { return row.ID },
		`SELECT student.id,student.display_name,mark.status,mark.mode,mark.updated_at`+attendanceOffering+attendancePlacements+attendanceMarks+attendanceDayEligibility+` AND ($3::uuid IS NULL OR student.id>$3) ORDER BY student.id LIMIT 51`, offering, date)
}
