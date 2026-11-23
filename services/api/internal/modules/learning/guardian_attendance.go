package learning

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/school-without-walls/api/internal/platform/response"
)

const linkedAttendanceChild = `
	FROM tenants t
	JOIN users u ON u.id=$2 AND u.status='ACTIVE'
	JOIN memberships m ON m.tenant_id=t.id AND m.user_id=u.id AND m.role='PARENT' AND m.status='ACTIVE'
	JOIN guardians guardian ON guardian.tenant_id=t.id AND guardian.user_id=u.id
	JOIN student_guardians link ON link.tenant_id=t.id AND link.guardian_id=guardian.id AND link.active
	JOIN students student ON student.tenant_id=t.id AND student.id=link.student_id AND student.status='ACTIVE'
	WHERE t.slug=$1::citext AND t.status='ACTIVE'
`

type attendanceChild struct {
	ID          uuid.UUID `json:"id"`
	DisplayName string    `json:"display_name"`
}

type guardianAttendanceMark struct {
	Date      string `json:"date"`
	ClassName string `json:"class_name"`
	Mode      string `json:"mode"`
	Status    string `json:"status"`
}

func (h *Handler) ListAttendanceChildren(w http.ResponseWriter, r *http.Request) {
	teacherPage(h, w, r, pgx.RowToStructByName[attendanceChild], func(row attendanceChild) uuid.UUID { return row.ID },
		`SELECT student.id,student.display_name`+linkedAttendanceChild+
			` AND ($3::uuid IS NULL OR student.id>$3) ORDER BY student.id LIMIT 51`)
}

func (h *Handler) GetChildAttendance(w http.ResponseWriter, r *http.Request) {
	actor, ok := teachingActor(w, r)
	if !ok {
		return
	}
	child, ok := teachingRouteID(w, r, "studentID")
	if !ok {
		return
	}
	var result struct {
		Child attendanceChild          `json:"child"`
		Marks []guardianAttendanceMark `json:"marks"`
	}
	err := h.pool.QueryRow(r.Context(), `SELECT student.id,student.display_name,ARRAY_TO_JSON(ARRAY(
		SELECT json_build_object('date',to_char(a.on_date,'YYYY-MM-DD'),'class_name',c.name,'mode',a.mode,'status',a.status)
		FROM attendance_records a JOIN classes c ON c.tenant_id=a.tenant_id AND c.id=a.class_id
		WHERE a.tenant_id=t.id AND a.student_id=student.id ORDER BY a.on_date DESC,a.id DESC LIMIT 50
	))`+linkedAttendanceChild+` AND student.id=$3`, chi.URLParam(r, "slug"), actor, child).
		Scan(&result.Child.ID, &result.Child.DisplayName, &result.Marks)
	if err != nil {
		lessonReadError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, result)
}
