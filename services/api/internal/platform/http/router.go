package http

import (
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/school-without-walls/api/internal/config"
	"github.com/school-without-walls/api/internal/modules/academics"
	"github.com/school-without-walls/api/internal/modules/auth"
	"github.com/school-without-walls/api/internal/modules/curriculum"
	"github.com/school-without-walls/api/internal/modules/people"
	"github.com/school-without-walls/api/internal/modules/tenants"
	"github.com/school-without-walls/api/internal/platform/db"
	"github.com/school-without-walls/api/internal/platform/middleware"
)

// RouterConfig holds dependencies for building the HTTP router.
type RouterConfig struct {
	Config         *config.Config
	DB             db.Pinger
	AuthRepo       auth.Repository
	TenantRepo     tenants.Repository
	AcademicRepo   academics.Repository
	StructureRepo  academics.StructureRepository
	SubjectRepo    academics.SubjectRepository
	OfferingRepo   academics.SubjectOfferingRepository
	AssignmentRepo academics.TeacherAssignmentRepository
}

// NewRouter constructs the Chi HTTP router with standard middleware and routes.
func NewRouter(cfg RouterConfig) (http.Handler, error) {
	if cfg.Config == nil {
		return nil, fmt.Errorf("router configuration is required")
	}

	r := chi.NewRouter()

	var peopleH *people.Handler
	var curriculumH *curriculum.Handler
	authRepo := cfg.AuthRepo
	tenantRepo := cfg.TenantRepo
	academicRepo := cfg.AcademicRepo
	structureRepo := cfg.StructureRepo
	subjectRepo := cfg.SubjectRepo
	offeringRepo := cfg.OfferingRepo
	assignmentRepo := cfg.AssignmentRepo
	if authRepo != nil || tenantRepo != nil || academicRepo != nil || structureRepo != nil || subjectRepo != nil || offeringRepo != nil || assignmentRepo != nil {
		if cfg.Config.Environment != "test" {
			return nil, fmt.Errorf("repository overrides are only allowed in the test environment")
		}
		if authRepo == nil || tenantRepo == nil || academicRepo == nil {
			return nil, fmt.Errorf("test routers require explicitly constructed repositories")
		}
	} else {
		if dbWrapper, ok := cfg.DB.(*db.DB); ok && dbWrapper != nil && dbWrapper.Pool != nil {
			authRepo = auth.NewPostgresRepository(dbWrapper.Pool)
			tenantRepo = tenants.NewPostgresRepositoryWithStaffLimits(dbWrapper.Pool, cfg.Config.StaffInvitations)
			academicRepo = academics.NewPostgresRepository(dbWrapper.Pool)
			structureRepo = academics.NewStructureRepository(dbWrapper.Pool)
			subjectRepo = academics.NewSubjectRepository(dbWrapper.Pool)
			offeringRepo = academics.NewSubjectOfferingRepository(dbWrapper.Pool)
			assignmentRepo = academics.NewTeacherAssignmentRepository(dbWrapper.Pool)
			peopleH = people.NewHandler(people.NewRepository(dbWrapper.Pool))
			curriculumH = curriculum.NewHandler(dbWrapper.Pool)
		} else {
			return nil, fmt.Errorf("authoritative PostgreSQL database is required")
		}
	}
	authSvc := auth.NewService(authRepo)
	isSecure := cfg.Config != nil && (cfg.Config.Environment == "production" || cfg.Config.Environment == "staging")
	var authH *auth.Handler
	var allowedOrigins []string
	var trustedProxyNets []*net.IPNet
	var mailer auth.Mailer
	var invitationMailer tenants.InvitationMailer
	var captureMailer *tenants.CaptureMailer
	if cfg.Config != nil {
		allowedOrigins = cfg.Config.CORSAllowedOrigins
		trustedProxyNets = cfg.Config.TrustedProxyNets
		if cfg.Config.SMTPHost != "" {
			smtpMailer := auth.NewSMTPMailer(cfg.Config.SMTPHost, cfg.Config.SMTPPort,
				cfg.Config.SMTPUsername, cfg.Config.SMTPPassword, cfg.Config.SMTPFrom, cfg.Config.PublicWebURL)
			mailer = smtpMailer
			invitationMailer = smtpMailer
		} else if cfg.Config.Environment == "development" || cfg.Config.Environment == "test" {
			captureMailer = tenants.NewCaptureMailer(cfg.Config.PublicWebURL)
			invitationMailer = captureMailer
		}
	}
	authH = auth.NewHandler(authSvc, isSecure, mailer, trustedProxyNets)
	tenantSvc := tenants.NewService(tenantRepo, invitationMailer)
	tenantH := tenants.NewHandler(tenantSvc, captureMailer)
	// Staff-specific invitations use the existing tenant repository
	// and the configured SMTP or local capture mailer.
	var staffInvitationH *tenants.StaffInvitationHandler
	if staffRepo, ok := tenantRepo.(tenants.StaffInvitationRepository); ok {
		if staffMailer, ok := invitationMailer.(tenants.StaffInvitationMailer); ok {
			staffInvitationH = tenants.NewStaffInvitationHandler(
				tenants.NewStaffInvitationService(staffRepo, staffMailer),
			)
		}
	}
	academicH := academics.NewHandler(academics.NewService(academicRepo))
	var structureH *academics.StructureHandler
	if structureRepo != nil {
		structureH = academics.NewStructureHandler(
			academics.NewStructureService(structureRepo),
		)
	}
	var subjectH *academics.SubjectHandler
	if subjectRepo != nil {
		subjectH = academics.NewSubjectHandler(academics.NewSubjectService(subjectRepo))
	}

	var offeringH *academics.SubjectOfferingHandler
	if offeringRepo != nil {
		offeringH = academics.NewSubjectOfferingHandler(
			academics.NewSubjectOfferingService(offeringRepo),
		)
	}

	var assignmentH *academics.TeacherAssignmentHandler
	if assignmentRepo != nil {
		assignmentH = academics.NewTeacherAssignmentHandler(
			academics.NewTeacherAssignmentService(assignmentRepo),
		)
	}

	// Base middleware stack
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)
	r.Use(middleware.Logger())
	if cfg.Config != nil {
		r.Use(auth.RequireSameOrigin(allowedOrigins))
		r.Use(corsMiddleware(allowedOrigins))
	}
	r.Use(auth.Authenticate(authSvc, isSecure))

	// Custom 404 / 405 matching standard JSON error envelopes
	r.NotFound(func(w http.ResponseWriter, req *http.Request) {
		ErrorJSON(w, req, http.StatusNotFound, ErrCodeNotFound, "Resource not found", map[string]any{
			"path": req.URL.Path,
		})
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, req *http.Request) {
		ErrorJSON(w, req, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", map[string]any{
			"method": req.Method,
		})
	})

	env := "development"
	if cfg.Config != nil {
		env = cfg.Config.Environment
	}
	healthH := NewHealthHandler(env)
	readyH := NewReadyHandler(cfg.DB)

	// Root operational probes
	r.Get("/health", healthH.Health)
	r.Get("/ready", readyH.Ready)

	// API v1 namespace
	r.Route("/api/v1", func(v1 chi.Router) {
		v1.Get("/health", healthH.Health)
		v1.Get("/ready", readyH.Ready)

		// Public authentication routes
		v1.Route("/auth", func(a chi.Router) {
			a.Post("/login", authH.Login)
			a.Post("/forgot-password", authH.ForgotPassword)
			a.Post("/reset-password", authH.ResetPassword)

			// Protected auth endpoints
			a.Group(func(protected chi.Router) {
				protected.Use(auth.RequireAuth)
				protected.Use(auth.RequireCSRF(authSvc))
				protected.Get("/me", authH.Me)
				protected.Post("/logout", authH.Logout)
				protected.Get("/sessions", authH.ListSessions)
				protected.Delete("/sessions/{id}", authH.RevokeSession)
			})
		})

		// Invitation endpoints
		v1.Route("/invitations", func(inv chi.Router) {
			inv.Get("/{token}/verify", authH.VerifyInvitation)
			inv.Post("/accept", authH.AcceptInvitation)
		})

		v1.Route("/platform", func(platform chi.Router) {
			platform.Group(func(protected chi.Router) {
				protected.Use(auth.RequireAuth)
				protected.Get("/tenants", tenantH.List)
				if captureMailer != nil {
					protected.Get("/mail-capture", tenantH.CapturedMail)
				}
				protected.Group(func(write chi.Router) {
					write.Use(auth.RequireCSRF(authSvc))
					write.Post("/tenants", tenantH.Provision)
				})
			})
		})

		v1.Group(func(protected chi.Router) {
			protected.Use(auth.RequireAuth)
			protected.Get("/my/schools", tenantH.ListMine)
			protected.Get("/tenants/{slug}/settings", tenantH.GetSettings)
			protected.With(auth.RequireCSRF(authSvc)).Patch("/tenants/{slug}/settings", tenantH.UpdateSettings)
			if staffInvitationH != nil {
				protected.With(auth.RequireCSRF(authSvc)).Post(
					"/tenants/{slug}/staff-invitations",
					staffInvitationH.Create,
				)
			}
		})

		if curriculumH != nil {
			v1.With(auth.RequireAuth).Get("/curriculum/frameworks", curriculumH.ListFrameworks)
			v1.With(auth.RequireAuth).Get("/curriculum/objectives", curriculumH.ListObjectives)
			v1.With(auth.RequireAuth).Get("/curriculum/alignments", curriculumH.ListVerifiedAlignments)
			v1.With(auth.RequireAuth, auth.RequireCSRF(authSvc)).Post("/curriculum/alignments", curriculumH.CreateAlignment)
			v1.With(auth.RequireAuth, auth.RequireCSRF(authSvc)).Post("/curriculum/alignments/{alignmentID}/review", curriculumH.ReviewAlignment)
		}

		if peopleH != nil {
			if staffInvitationH != nil {
				v1.With(auth.RequireAuth, auth.RequireCSRF(authSvc)).Post("/tenants/{slug}/students/{studentID}/invitations", staffInvitationH.CreateStudent)
				v1.With(auth.RequireAuth, auth.RequireCSRF(authSvc)).Post("/tenants/{slug}/guardians/{guardianID}/invitations", staffInvitationH.CreateGuardian)
			}
			v1.With(auth.RequireAuth).Get("/tenants/{slug}/staff", peopleH.ListStaff)
			v1.With(auth.RequireAuth).Get("/tenants/{slug}/staff/candidates", peopleH.ListStaffCandidates)
			v1.With(auth.RequireAuth, auth.RequireCSRF(authSvc)).Post("/tenants/{slug}/staff", peopleH.CreateStaff)
			v1.With(auth.RequireAuth, auth.RequireCSRF(authSvc)).Put("/tenants/{slug}/staff/{staffID}", peopleH.UpdateStaff)
			v1.With(auth.RequireAuth).Get("/tenants/{slug}/students", peopleH.ListStudents)
			v1.With(auth.RequireAuth, auth.RequireCSRF(authSvc)).Post("/tenants/{slug}/students", peopleH.CreateStudent)
			v1.With(auth.RequireAuth).Get("/tenants/{slug}/students/{studentID}", peopleH.GetStudent)
			v1.With(auth.RequireAuth, auth.RequireCSRF(authSvc)).Put("/tenants/{slug}/students/{studentID}", peopleH.UpdateStudent)
			v1.With(auth.RequireAuth).Get("/tenants/{slug}/guardians", peopleH.ListGuardians)
			v1.With(auth.RequireAuth, auth.RequireCSRF(authSvc)).Post("/tenants/{slug}/guardians", peopleH.CreateGuardian)
			v1.With(auth.RequireAuth, auth.RequireCSRF(authSvc)).Put("/tenants/{slug}/guardians/{guardianID}", peopleH.UpdateGuardian)
			v1.With(auth.RequireAuth, auth.RequireCSRF(authSvc)).Put("/tenants/{slug}/students/{studentID}/guardians/{guardianID}", peopleH.LinkGuardian)
			v1.With(auth.RequireAuth, auth.RequireCSRF(authSvc)).Delete("/tenants/{slug}/students/{studentID}/guardians/{guardianID}", peopleH.UnlinkGuardian)
			v1.With(auth.RequireAuth).Get("/tenants/{slug}/students/{studentID}/enrollments", peopleH.ListEnrollments)
			v1.With(auth.RequireAuth, auth.RequireCSRF(authSvc)).Post("/tenants/{slug}/students/{studentID}/enrollments", peopleH.CreateEnrollment)
			v1.With(auth.RequireAuth, auth.RequireCSRF(authSvc)).Post("/tenants/{slug}/students/{studentID}/enrollments/{enrollmentID}/transfers", peopleH.TransferEnrollment)
		}
		v1.With(auth.RequireAuth).Get("/tenants/{slug}/academic-sessions", academicH.ListSessions)
		v1.With(auth.RequireAuth, auth.RequireCSRF(authSvc)).Post("/tenants/{slug}/academic-sessions", academicH.CreateSession)
		v1.With(auth.RequireAuth).Get("/tenants/{slug}/academic-sessions/{sessionID}", academicH.GetSession)
		v1.With(auth.RequireAuth, auth.RequireCSRF(authSvc)).Patch("/tenants/{slug}/academic-sessions/{sessionID}", academicH.UpdateSession)
		v1.With(auth.RequireAuth).Get("/tenants/{slug}/academic-sessions/{sessionID}/terms", academicH.GetTerms)
		v1.With(auth.RequireAuth, auth.RequireCSRF(authSvc)).Put("/tenants/{slug}/academic-sessions/{sessionID}/terms", academicH.ConfigureTerms)
		v1.With(auth.RequireAuth, auth.RequireCSRF(authSvc)).Post("/tenants/{slug}/academic-sessions/{sessionID}/activate", academicH.ActivateSession)
		v1.With(auth.RequireAuth, auth.RequireCSRF(authSvc)).Post("/tenants/{slug}/academic-sessions/{sessionID}/archive", academicH.ArchiveSession)

		if subjectH != nil {
			v1.With(auth.RequireAuth).Get("/tenants/{slug}/subjects", subjectH.ListSubjects)
			v1.With(auth.RequireAuth, auth.RequireCSRF(authSvc)).Post("/tenants/{slug}/subjects", subjectH.CreateSubject)
			v1.With(auth.RequireAuth, auth.RequireCSRF(authSvc)).Patch("/tenants/{slug}/subjects/{subjectID}", subjectH.UpdateSubject)
			v1.With(auth.RequireAuth, auth.RequireCSRF(authSvc)).Post("/tenants/{slug}/subjects/{subjectID}/archive", subjectH.ArchiveSubject)
		}
		if offeringH != nil {
			v1.With(auth.RequireAuth).Get("/tenants/{slug}/subject-offerings", offeringH.ListSubjectOfferings)
			v1.With(auth.RequireAuth, auth.RequireCSRF(authSvc)).Post("/tenants/{slug}/subject-offerings", offeringH.CreateSubjectOffering)
			v1.With(auth.RequireAuth, auth.RequireCSRF(authSvc)).Post("/tenants/{slug}/subject-offerings/{offeringID}/archive", offeringH.ArchiveSubjectOffering)
		}

		if assignmentH != nil {
			v1.With(auth.RequireAuth).Get("/tenants/{slug}/teacher-assignments", assignmentH.ListTeacherAssignments)
			v1.With(auth.RequireAuth, auth.RequireCSRF(authSvc)).Post("/tenants/{slug}/teacher-assignments", assignmentH.CreateTeacherAssignment)
			v1.With(auth.RequireAuth, auth.RequireCSRF(authSvc)).Post("/tenants/{slug}/teacher-assignments/{assignmentID}/deactivate", assignmentH.DeactivateTeacherAssignment)
		}

		if structureH != nil {
			v1.With(auth.RequireAuth).Get("/tenants/{slug}/grade-levels", structureH.ListGradeLevels)
			v1.With(auth.RequireAuth, auth.RequireCSRF(authSvc)).Post("/tenants/{slug}/grade-levels", structureH.CreateGradeLevel)

			v1.With(auth.RequireAuth).Get("/tenants/{slug}/classes", structureH.ListClasses)
			v1.With(auth.RequireAuth, auth.RequireCSRF(authSvc)).Post("/tenants/{slug}/classes", structureH.CreateClass)

			v1.With(auth.RequireAuth).Get("/tenants/{slug}/class-arms", structureH.ListClassArms)
			v1.With(auth.RequireAuth, auth.RequireCSRF(authSvc)).Post("/tenants/{slug}/class-arms", structureH.CreateClassArm)

			v1.With(auth.RequireAuth, auth.RequireCSRF(authSvc)).Patch("/tenants/{slug}/grade-levels/{gradeLevelID}", structureH.UpdateGradeLevel)
			v1.With(auth.RequireAuth, auth.RequireCSRF(authSvc)).Post("/tenants/{slug}/grade-levels/{gradeLevelID}/archive", structureH.ArchiveGradeLevel)

			v1.With(auth.RequireAuth, auth.RequireCSRF(authSvc)).Patch("/tenants/{slug}/classes/{classID}", structureH.UpdateClass)
			v1.With(auth.RequireAuth, auth.RequireCSRF(authSvc)).Post("/tenants/{slug}/classes/{classID}/archive", structureH.ArchiveClass)

			v1.With(auth.RequireAuth, auth.RequireCSRF(authSvc)).Patch("/tenants/{slug}/class-arms/{armID}", structureH.UpdateClassArm)
			v1.With(auth.RequireAuth, auth.RequireCSRF(authSvc)).Post("/tenants/{slug}/class-arms/{armID}/archive", structureH.ArchiveClassArm)
		}
	})

	return r, nil
}

func corsMiddleware(allowedOrigins []string) func(http.Handler) http.Handler {
	allowedMap := make(map[string]bool)
	for _, o := range allowedOrigins {
		allowedMap[strings.TrimRight(o, "/")] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" && allowedMap[strings.TrimRight(origin, "/")] {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, X-Request-ID, X-CSRF-Token")
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Access-Control-Max-Age", "300")
			}

			if strings.EqualFold(r.Method, http.MethodOptions) {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
