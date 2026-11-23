package learning

import (
	"net/http"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/school-without-walls/api/internal/platform/middleware"
	"github.com/school-without-walls/api/internal/platform/response"
)

type attendanceChange struct {
	StudentID         uuid.UUID  `json:"student_id"`
	Status            string     `json:"status"`
	Mode              string     `json:"mode"`
	ExpectedUpdatedAt *time.Time `json:"expected_updated_at"`
	Reason            string     `json:"reason"`
}

type attendancePlacement struct {
	StudentID    uuid.UUID
	EnrollmentID uuid.UUID
	RecordID     *uuid.UUID
	Status       *string
	Mode         *string
	UpdatedAt    *time.Time
}

func (h *Handler) SaveAttendance(w http.ResponseWriter, r *http.Request) {
	actor, ok := teachingActor(w, r)
	if !ok {
		return
	}
	offering, ok := teachingRouteID(w, r, "offeringID")
	if !ok {
		return
	}
	var batch struct {
		Date  string             `json:"date"`
		Marks []attendanceChange `json:"marks"`
	}
	if !decodeLesson(w, r, &batch) {
		invalidAttendance(w, r)
		return
	}
	date, err := time.Parse(time.DateOnly, batch.Date)
	if err != nil || date.Year() < 1 || len(batch.Marks) == 0 || len(batch.Marks) > 50 {
		invalidAttendance(w, r)
		return
	}
	ids := make([]uuid.UUID, 0, len(batch.Marks))
	seen := make(map[uuid.UUID]bool, len(batch.Marks))
	for i := range batch.Marks {
		mark := &batch.Marks[i]
		mark.Reason = strings.TrimSpace(mark.Reason)
		if seen[mark.StudentID] || mark.StudentID == uuid.Nil ||
			!slices.Contains([]string{"ON_SITE", "REMOTE"}, mark.Mode) ||
			!slices.Contains([]string{"PRESENT", "ABSENT", "LATE", "EXCUSED"}, mark.Status) ||
			utf8.RuneCountInString(mark.Reason) > 500 || strings.ContainsRune(mark.Reason, 0) {
			invalidAttendance(w, r)
			return
		}
		seen[mark.StudentID] = true
		ids = append(ids, mark.StudentID)
	}

	ctx := r.Context()
	tx, err := h.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		lessonReadError(w, r, err)
		return
	}
	defer tx.Rollback(ctx)
	params := []any{chi.URLParam(r, "slug"), actor, ids, offering, date}
	var tenant, member, session, term, class uuid.UUID
	scope := `SELECT t.id,m.id,sess.id,term.id,c.id` + attendanceOffering + attendanceOfferingEligibility + ` AND $3::uuid[] IS NOT NULL`
	err = tx.QueryRow(ctx, scope, params...).Scan(&tenant, &member, &session, &term, &class)
	if err != nil {
		lessonReadError(w, r, err)
		return
	}
	// Different subject offerings for the same class share the same register.
	// Serialize their writes before acquiring student locks in stable ID order.
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "sww.attendance:"+tenant.String()+":"+class.String()); err != nil {
		lessonReadError(w, r, err)
		return
	}
	err = tx.QueryRow(ctx, scope+` FOR SHARE OF t,u,m,a,o,term,sess,c,g`, params...).Scan(&tenant, &member, &session, &term, &class)
	if err != nil {
		lessonReadError(w, r, err)
		return
	}
	rows, err := tx.Query(ctx, `SELECT id FROM students WHERE tenant_id=$1 AND id=ANY($2::uuid[]) ORDER BY id FOR UPDATE`, tenant, ids)
	if err != nil {
		lessonReadError(w, r, err)
		return
	}
	locked, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		lessonReadError(w, r, err)
		return
	}
	if len(locked) != len(ids) {
		lessonUnavailable(w, r)
		return
	}

	rows, err = tx.Query(ctx, `SELECT student.id,placement.id,mark.id,mark.status,mark.mode,mark.updated_at`+
		attendanceOffering+attendancePlacements+attendanceMarks+attendanceDayEligibility+` AND student.id=ANY($3::uuid[])`, params...)
	if err != nil {
		lessonReadError(w, r, err)
		return
	}
	placements, err := pgx.CollectRows(rows, pgx.RowToStructByPos[attendancePlacement])
	if err != nil {
		lessonReadError(w, r, err)
		return
	}
	if len(placements) != len(ids) {
		lessonUnavailable(w, r)
		return
	}
	byStudent := make(map[uuid.UUID]attendancePlacement, len(placements))
	for _, p := range placements {
		byStudent[p.StudentID] = p
	}
	changed := 0
	for _, mark := range batch.Marks {
		p := byStudent[mark.StudentID]
		if p.RecordID != nil && *p.Status == mark.Status && *p.Mode == mark.Mode {
			continue
		}
		if (p.UpdatedAt == nil) != (mark.ExpectedUpdatedAt == nil) ||
			(p.UpdatedAt != nil && !p.UpdatedAt.Equal(*mark.ExpectedUpdatedAt)) {
			response.ErrorJSON(w, r, http.StatusConflict, response.ErrCodeConflict, "Attendance changed. Reload the roster before saving.", nil)
			return
		}
		var id uuid.UUID
		if p.RecordID == nil {
			id = uuid.New()
			_, err = tx.Exec(ctx, `INSERT INTO attendance_records (id,tenant_id,enrollment_id,student_id,session_id,term_id,class_id,on_date,mode,status,recorded_by)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, id, tenant, p.EnrollmentID, mark.StudentID, session, term, class, date, mark.Mode, mark.Status, member)
		} else {
			if mark.Reason == "" {
				invalidAttendance(w, r)
				return
			}
			id = *p.RecordID
			_, err = tx.Exec(ctx, `UPDATE attendance_records SET status=$3,mode=$4,updated_at=clock_timestamp()
				WHERE tenant_id=$1 AND id=$2`, tenant, id, mark.Status, mark.Mode)
		}
		if err == nil {
			_, err = tx.Exec(ctx, `INSERT INTO audit_events (id,tenant_id,actor_id,action,subject_type,subject_id,request_id,metadata_redacted)
				VALUES ($1,$2,$3,'attendance.mark.save','attendance_record',$4,$5,jsonb_build_object(
					'previous_status',$6::text,'previous_mode',$7::text,'status',$8::text,'mode',$9::text,'reason',NULLIF($10::text,'')))`,
				uuid.New(), tenant, actor, id.String(), middleware.GetRequestID(ctx), p.Status, p.Mode, mark.Status, mark.Mode, mark.Reason)
		}
		if err != nil {
			lessonReadError(w, r, err)
			return
		}
		changed++
	}
	if err = tx.Commit(ctx); err != nil {
		lessonReadError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]int{"changed": changed})
}

func invalidAttendance(w http.ResponseWriter, r *http.Request) {
	response.ErrorJSON(w, r, http.StatusUnprocessableEntity, response.ErrCodeInvalidInput, "Check attendance marks and provide a reason for every correction", nil)
}
