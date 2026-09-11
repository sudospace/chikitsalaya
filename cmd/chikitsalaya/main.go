// Command chikitsalaya runs the Chikitsalaya clinic web app (OPD module for
// now; more clinical modules land as siblings under internal/ later).
package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os/signal"
	"strings"
	"syscall"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"chikitsalaya/internal/auth"
	"chikitsalaya/internal/config"
	"chikitsalaya/internal/db"
	"chikitsalaya/internal/opd/account"
	"chikitsalaya/internal/opd/appointment"
	"chikitsalaya/internal/opd/billing"
	"chikitsalaya/internal/opd/booking"
	"chikitsalaya/internal/opd/company"
	"chikitsalaya/internal/opd/dashboard"
	"chikitsalaya/internal/opd/encounter"
	"chikitsalaya/internal/opd/integrations"
	"chikitsalaya/internal/opd/inventory"
	"chikitsalaya/internal/opd/masters"
	"chikitsalaya/internal/opd/notify"
	"chikitsalaya/internal/opd/onboarding"
	"chikitsalaya/internal/opd/patient"
	"chikitsalaya/internal/opd/practitioner"
	"chikitsalaya/internal/opd/schedule"
	"chikitsalaya/internal/opd/staff"
	"chikitsalaya/internal/web"
)

func main() {
	seedAdmin := flag.Bool("seed-admin", false, "create/reset the clinic admin user and exit")
	adminEmail := flag.String("admin-email", "", "admin email (with --seed-admin)")
	adminPassword := flag.String("admin-password", "", "admin password (with --seed-admin)")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg := config.Load()

	if err := db.Migrate(cfg.DatabaseURL); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("connect db: %v", err)
	}
	defer pool.Close()

	userStore := auth.NewStore(pool)
	permStore := auth.NewPermissionStore(pool)
	companyStore := company.NewStore(pool)

	if *seedAdmin {
		if *adminEmail == "" || *adminPassword == "" {
			log.Fatal("--admin-email and --admin-password are required with --seed-admin")
		}
		adminID, err := userStore.UpsertAdmin(ctx, *adminEmail, *adminPassword)
		if err != nil {
			log.Fatalf("seed admin: %v", err)
		}
		if _, err := companyStore.EnsureClinic(ctx); err != nil {
			log.Fatalf("ensure clinic: %v", err)
		}
		if err := permStore.SetAll(ctx, adminID, auth.AdminDefaultPermissions); err != nil {
			log.Fatalf("grant admin permissions: %v", err)
		}
		log.Printf("admin user %s ready", *adminEmail)
		return
	}

	renderer, err := web.NewRenderer()
	if err != nil {
		log.Fatalf("load templates: %v", err)
	}

	sm := auth.NewSessionManager(pool)
	authHandlers := auth.NewHandlers(userStore, sm, renderer)

	patientStore := patient.NewStore(pool)
	patientHandlers := patient.NewHandlers(patientStore, renderer)

	companyHandlers := company.NewHandlers(companyStore, sm, renderer)

	// Base for Chrome.RoleLabel below, which appends " + Doctor" while an
	// admin is acting as their linked practitioner.
	roleLabels := map[string]string{
		auth.RoleAdmin:        "Admin",
		auth.RoleDoctor:       "Doctor",
		auth.RoleReceptionist: "Receptionist",
	}

	renderer.SetChromeResolver(func(r *http.Request) web.Chrome {
		role := auth.CurrentRole(sm, r)
		actingAsDoctor := auth.IsActingAsDoctor(sm, r)
		userID := auth.CurrentUserID(sm, r)
		canBilling, _ := permStore.HasAccess(r.Context(), role, userID, auth.ModuleBilling, auth.AccessView)
		chrome := web.Chrome{
			Role:             role,
			IsAdmin:          role == auth.RoleAdmin,
			IsActingAsDoctor: actingAsDoctor,
			RoleLabel:        roleLabels[role],
			CanBilling:       canBilling,
		}
		if chrome.RoleLabel == "" {
			chrome.RoleLabel = role
		}
		if role == auth.RoleAdmin && actingAsDoctor {
			chrome.RoleLabel += " + Doctor"
		}
		if c, err := companyStore.GetClinic(r.Context()); err == nil {
			chrome.ClinicName = c.Name
		}
		if userID != 0 {
			if u, err := userStore.GetByID(r.Context(), userID); err == nil {
				chrome.UserEmail = u.Email
				if len(u.Email) > 0 {
					chrome.UserInitial = strings.ToUpper(u.Email[:1])
				}
			}
		}
		return chrome
	})

	scheduleStore := schedule.NewStore(pool)
	scheduleHandlers := schedule.NewHandlers(scheduleStore)

	practitionerStore := practitioner.NewStore(pool)
	practitionerHandlers := practitioner.NewHandlers(practitionerStore, scheduleStore, companyStore, sm, renderer)

	appointmentStore := appointment.NewStore(pool)
	appointmentHandlers := appointment.NewHandlers(appointmentStore, patientStore, practitionerStore, scheduleStore, companyStore, sm, renderer)

	inventoryStore := inventory.NewStore(pool)
	inventoryHandlers := inventory.NewHandlers(inventoryStore, sm, renderer)

	mastersStore := masters.NewStore(pool)
	mastersHandlers := masters.NewHandlers(mastersStore, sm, renderer)

	staffHandlers := staff.NewHandlers(userStore, permStore, practitionerStore, sm, renderer)

	onboardingHandlers := onboarding.NewHandlers(userStore, practitionerStore, scheduleStore, inventoryStore, mastersStore, sm, renderer)

	encounterStore := encounter.NewStore(pool)
	encounterHandlers := encounter.NewHandlers(encounterStore, appointmentStore, practitionerStore, inventoryStore, mastersStore, companyStore, patientStore, userStore, permStore, sm, renderer)

	integrationsStore := integrations.NewStore(pool)
	integrationsHandlers := integrations.NewHandlers(integrationsStore, sm, renderer)

	otpStore := booking.NewOTPStore(pool)
	bookingHandlers := booking.NewHandlers(companyStore, practitionerStore, scheduleStore, appointmentStore, patientStore,
		otpStore, integrationsStore, notify.ConsoleSender{}, sm, renderer)

	dashboardHandlers := dashboard.NewHandlers(appointmentStore, companyStore, patientStore, practitionerStore, permStore, userStore, sm, renderer)

	accountHandlers := account.NewHandlers(userStore, permStore, practitionerStore, scheduleStore, companyStore, sm, renderer)

	billingStore := billing.NewStore(pool)
	billingHandlers := billing.NewHandlers(billingStore, encounterStore, appointmentStore, patientStore,
		practitionerStore, companyStore, mastersStore, inventoryStore, userStore, sm, renderer)

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(sm.LoadAndSave)

	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.Dir("web/static"))))

	// Public: a 404 here just means "no logo", handled by the caller.
	r.Get("/branding/logo", companyHandlers.LogoHandler)
	r.Get("/favicon.ico", companyHandlers.LogoHandler)
	r.Get("/branding/letterhead", companyHandlers.LetterheadImageHandler)

	r.Get("/login", authHandlers.LoginPage)
	r.Post("/login", authHandlers.Login)
	r.Post("/logout", authHandlers.Logout)

	// Public online booking — one clinic, no slug needed.
	r.Route("/book", func(bk chi.Router) {
		bk.Get("/", bookingHandlers.Index)
		bk.Post("/", bookingHandlers.Create)
		bk.Get("/slots", bookingHandlers.Slots)
		bk.Post("/otp/request", bookingHandlers.RequestOTP)
		bk.Post("/otp/verify", bookingHandlers.VerifyOTP)
		bk.Post("/logout", bookingHandlers.Logout)
		bk.Post("/appointments/{id}/cancel", bookingHandlers.Cancel)
		bk.Get("/appointments/{id}/reschedule", bookingHandlers.RescheduleForm)
		bk.Get("/appointments/{id}/reschedule/slots", bookingHandlers.RescheduleSlots)
		bk.Post("/appointments/{id}/reschedule", bookingHandlers.Reschedule)
	})

	// Reachable only with a password-verified-but-not-yet-2FA'd session.
	r.Group(func(tr chi.Router) {
		tr.Use(auth.RequirePending2FA(sm))
		tr.Get("/2fa/setup", authHandlers.TwoFASetupPage)
		tr.Post("/2fa/setup", authHandlers.TwoFASetupConfirm)
		tr.Get("/2fa/verify", authHandlers.TwoFAVerifyPage)
		tr.Post("/2fa/verify", authHandlers.TwoFAVerifyConfirm)
	})

	// view() gates GET/list/detail routes; edit() also gates routes that
	// change something.
	view := func(module string) func(http.Handler) http.Handler {
		return auth.RequireModule(sm, permStore, module, auth.AccessView)
	}
	edit := func(module string) func(http.Handler) http.Handler {
		return auth.RequireModule(sm, permStore, module, auth.AccessEdit)
	}

	r.Group(func(pr chi.Router) {
		pr.Use(auth.RequireAuth(sm))

		pr.Get("/", dashboardHandlers.Home)

		// Self-service account page — any logged-in user, own row only.
		pr.Get("/account", accountHandlers.Page)
		pr.Post("/account/password", accountHandlers.UpdatePassword)
		pr.Post("/account/personal", accountHandlers.UpdatePersonal)
		pr.Post("/account/schedule", accountHandlers.AddSchedule)
		pr.Post("/account/schedule/{id}/delete", accountHandlers.DeleteSchedule)
		pr.Post("/account/link-practitioner", accountHandlers.LinkPractitioner)
		pr.Post("/account/act-as-doctor", accountHandlers.ActAsDoctor)
		pr.Post("/account/stop-acting-as-doctor", accountHandlers.StopActingAsDoctor)

		// One-time onboarding wizard.
		pr.Get("/onboarding", onboardingHandlers.Index)
		pr.Post("/onboarding/skip", onboardingHandlers.Skip)
		pr.Get("/onboarding/practitioners", onboardingHandlers.PractitionersStep)
		pr.Post("/onboarding/practitioners", onboardingHandlers.CreatePractitionerStep)
		pr.Post("/onboarding/practitioners/{id}/schedules", onboardingHandlers.CreatePractitionerSchedule)
		pr.Get("/onboarding/drugs", onboardingHandlers.DrugsStep)
		pr.Post("/onboarding/drugs", onboardingHandlers.CreateDrugStep)
		pr.Get("/onboarding/lab-tests", onboardingHandlers.LabTestsStep)
		pr.Post("/onboarding/lab-tests", onboardingHandlers.CreateLabTestStep)
		pr.Get("/onboarding/procedures", onboardingHandlers.ProceduresStep)
		pr.Post("/onboarding/procedures", onboardingHandlers.CreateProcedureStep)
		pr.Post("/onboarding/finish", onboardingHandlers.Finish)

		pr.Get("/search", dashboardHandlers.Search)

		pr.Route("/admin", func(ar chi.Router) {
			ar.Use(auth.RequireRole(sm, auth.RoleAdmin))

			ar.With(view(auth.ModuleCompanySettings)).Get("/company", companyHandlers.MyCompany)
			ar.With(edit(auth.ModuleCompanySettings)).Post("/company", companyHandlers.UpdateMyCompany)

			ar.With(view(auth.ModuleIntegrations)).Get("/integrations", integrationsHandlers.Index)
			ar.With(edit(auth.ModuleIntegrations)).Post("/integrations/sms", integrationsHandlers.UpdateSMS)
			ar.With(edit(auth.ModuleIntegrations)).Post("/integrations/email", integrationsHandlers.UpdateEmail)
			ar.With(edit(auth.ModuleIntegrations)).Post("/integrations/payment", integrationsHandlers.UpdatePayment)

			ar.Route("/service-units", func(sr chi.Router) {
				sr.Use(view(auth.ModuleServiceUnits))
				sr.Get("/", companyHandlers.ListServiceUnits)
				sr.With(edit(auth.ModuleServiceUnits)).Get("/new", companyHandlers.NewServiceUnit)
				sr.With(edit(auth.ModuleServiceUnits)).Post("/", companyHandlers.CreateServiceUnit)
				sr.With(edit(auth.ModuleServiceUnits)).Get("/{id}/edit", companyHandlers.EditServiceUnit)
				sr.With(edit(auth.ModuleServiceUnits)).Post("/{id}", companyHandlers.UpdateServiceUnit)
			})

			ar.Route("/drugs", func(dr chi.Router) {
				dr.Use(view(auth.ModuleDrugs))
				dr.Get("/", inventoryHandlers.List)
				dr.With(edit(auth.ModuleDrugs)).Get("/new", inventoryHandlers.New)
				dr.With(edit(auth.ModuleDrugs)).Post("/", inventoryHandlers.Create)
				dr.With(edit(auth.ModuleDrugs)).Get("/{id}/edit", inventoryHandlers.Edit)
				dr.With(edit(auth.ModuleDrugs)).Post("/{id}", inventoryHandlers.Update)
			})

			ar.Route("/lookup-values", func(lr chi.Router) {
				lr.Use(view(auth.ModuleLookupValues))
				lr.Get("/", mastersHandlers.ListLookupValues)
				lr.With(edit(auth.ModuleLookupValues)).Get("/new", mastersHandlers.NewLookupValue)
				lr.With(edit(auth.ModuleLookupValues)).Post("/", mastersHandlers.CreateLookupValue)
				lr.With(edit(auth.ModuleLookupValues)).Get("/{id}/edit", mastersHandlers.EditLookupValue)
				lr.With(edit(auth.ModuleLookupValues)).Post("/{id}", mastersHandlers.UpdateLookupValue)
				lr.With(edit(auth.ModuleLookupValues)).Post("/{id}/delete", mastersHandlers.DeleteLookupValue)
			})

			ar.Route("/lab-tests", func(tr chi.Router) {
				tr.Use(view(auth.ModuleLabTests))
				tr.Get("/", mastersHandlers.ListLabTestTemplates)
				tr.With(edit(auth.ModuleLabTests)).Get("/new", mastersHandlers.NewLabTestTemplate)
				tr.With(edit(auth.ModuleLabTests)).Post("/", mastersHandlers.CreateLabTestTemplate)
				tr.With(edit(auth.ModuleLabTests)).Get("/{id}/edit", mastersHandlers.EditLabTestTemplate)
				tr.With(edit(auth.ModuleLabTests)).Post("/{id}", mastersHandlers.UpdateLabTestTemplate)
			})

			ar.Route("/procedures", func(pcr chi.Router) {
				pcr.Use(view(auth.ModuleProcedures))
				pcr.Get("/", mastersHandlers.ListProcedureTemplates)
				pcr.With(edit(auth.ModuleProcedures)).Get("/new", mastersHandlers.NewProcedureTemplate)
				pcr.With(edit(auth.ModuleProcedures)).Post("/", mastersHandlers.CreateProcedureTemplate)
				pcr.With(edit(auth.ModuleProcedures)).Get("/{id}/edit", mastersHandlers.EditProcedureTemplate)
				pcr.With(edit(auth.ModuleProcedures)).Post("/{id}", mastersHandlers.UpdateProcedureTemplate)
			})

			ar.Route("/users", func(ur chi.Router) {
				ur.Use(view(auth.ModuleUsers))
				ur.Get("/", staffHandlers.List)
				ur.With(edit(auth.ModuleUsers)).Get("/new", staffHandlers.New)
				ur.With(edit(auth.ModuleUsers)).Post("/", staffHandlers.Create)
				ur.With(edit(auth.ModuleUsers)).Get("/{id}/edit", staffHandlers.Edit)
				ur.With(edit(auth.ModuleUsers)).Post("/{id}", staffHandlers.Update)
				ur.With(edit(auth.ModuleUsers)).Post("/{id}/toggle-active", staffHandlers.ToggleActive)
			})

			// No view level — managing practitioners is inherently an edit
			// action (the read-only directory is plain /practitioners below).
			ar.Route("/practitioners-manage", func(pmr chi.Router) {
				pmr.Use(edit(auth.ModulePractitioners))
				pmr.Get("/new", practitionerHandlers.New)
				pmr.Post("/", practitionerHandlers.Create)
				pmr.Get("/{id}/edit", practitionerHandlers.Edit)
				pmr.Post("/{id}", practitionerHandlers.Update)
				pmr.Post("/{id}/schedules", scheduleHandlers.Create)
				pmr.Post("/{id}/schedules/{scheduleID}/delete", scheduleHandlers.Delete)
			})
		})

		pr.Route("/patients", func(pt chi.Router) {
			pt.Use(view(auth.ModulePatients))
			pt.Get("/", patientHandlers.List)
			pt.Get("/search", patientHandlers.Search)
			pt.With(edit(auth.ModulePatients)).Get("/new", patientHandlers.New)
			pt.With(edit(auth.ModulePatients)).Post("/", patientHandlers.Create)
			pt.Get("/{id}", patientHandlers.Detail)
			pt.With(edit(auth.ModulePatients)).Get("/{id}/edit", patientHandlers.Edit)
			pt.With(edit(auth.ModulePatients)).Post("/{id}", patientHandlers.Update)
			pt.Get("/{id}/history", encounterHandlers.PatientHistory)
		})

		// Managing practitioners (new/edit/schedules) needs edit-level,
		// under /admin/practitioners-manage above.
		pr.Route("/practitioners", func(pc chi.Router) {
			pc.Use(view(auth.ModulePractitioners))
			pc.Get("/", practitionerHandlers.List)
			pc.Get("/{id}", practitionerHandlers.Detail)
		})

		pr.Route("/appointments", func(ap chi.Router) {
			ap.Use(view(auth.ModuleAppointments))
			ap.Get("/", appointmentHandlers.Index)
			ap.Get("/new", appointmentHandlers.New)
			ap.Get("/new/search", appointmentHandlers.SearchPatients)
			ap.Get("/slots", appointmentHandlers.Slots)
			ap.With(edit(auth.ModuleAppointments)).Post("/", appointmentHandlers.Create)
			ap.With(edit(auth.ModuleAppointments)).Post("/{id}/status", appointmentHandlers.UpdateStatus)
			ap.With(edit(auth.ModuleAppointments)).Post("/{id}/fee", appointmentHandlers.SetFee)
			ap.With(edit(auth.ModulePatients)).Post("/new/patients", appointmentHandlers.CreatePatientForBooking)
		})

		pr.Route("/catalog", func(cr chi.Router) {
			cr.Use(view(auth.ModuleEncounters))
			cr.Get("/drugs-search", encounterHandlers.SearchDrugsHandler)
			cr.Get("/lab-tests-search", encounterHandlers.SearchLabTestsHandler)
			cr.Get("/procedures-search", encounterHandlers.SearchProceduresHandler)
			cr.With(edit(auth.ModuleEncounters)).Post("/quick-add-drug", encounterHandlers.QuickAddDrugHandler)
			cr.With(edit(auth.ModuleEncounters)).Post("/quick-add-lab-test", encounterHandlers.QuickAddLabTestHandler)
			cr.With(edit(auth.ModuleEncounters)).Post("/quick-add-procedure", encounterHandlers.QuickAddProcedureHandler)
			cr.With(edit(auth.ModuleEncounters)).Post("/quick-add-lookup-value", encounterHandlers.QuickAddLookupValueHandler)
		})

		pr.With(edit(auth.ModuleEncounters)).Get("/quick-prescription", encounterHandlers.QuickPrescriptionForm)
		pr.With(edit(auth.ModuleEncounters)).Get("/quick-prescription/patients", encounterHandlers.QuickPrescriptionSearchPatients)
		pr.With(edit(auth.ModuleEncounters)).Post("/quick-prescription", encounterHandlers.CreateQuickPrescription)

		pr.With(edit(auth.ModuleEncounters)).Post("/encounters/from-appointment/{id}", encounterHandlers.StartConsultation)
		pr.Route("/encounters/{id}", func(er chi.Router) {
			er.Use(view(auth.ModuleEncounters))
			er.Get("/", encounterHandlers.Workspace)
			er.With(edit(auth.ModuleEncounters)).Post("/", encounterHandlers.UpdateCore)
			er.With(edit(auth.ModuleEncounters)).Post("/vitals", encounterHandlers.UpsertVitalsHandler)
			er.Get("/icd10-search", encounterHandlers.SearchICD10Handler)
			er.With(edit(auth.ModuleEncounters)).Post("/diagnoses", encounterHandlers.AddDiagnosisHandler)
			er.With(edit(auth.ModuleEncounters)).Post("/diagnoses/{diagID}/delete", encounterHandlers.DeleteDiagnosisHandler)
			er.With(edit(auth.ModuleEncounters)).Post("/prescription-items", encounterHandlers.AddPrescriptionItemHandler)
			er.With(edit(auth.ModuleEncounters)).Post("/prescription-items/{itemID}/delete", encounterHandlers.DeletePrescriptionItemHandler)
			er.With(edit(auth.ModuleEncounters)).Post("/quick-add-drug", encounterHandlers.QuickAddDrugHandler)
			er.With(edit(auth.ModuleEncounters)).Post("/lab-orders", encounterHandlers.AddLabOrderHandler)
			er.With(edit(auth.ModuleEncounters)).Post("/lab-orders/{labID}/result", encounterHandlers.RecordLabResultHandler)
			er.With(edit(auth.ModuleEncounters)).Post("/lab-orders/{labID}/delete", encounterHandlers.DeleteLabOrderHandler)
			er.With(edit(auth.ModuleEncounters)).Post("/quick-add-lab-test", encounterHandlers.QuickAddLabTestHandler)
			er.With(edit(auth.ModuleEncounters)).Post("/procedure-orders", encounterHandlers.AddProcedureOrderHandler)
			er.With(edit(auth.ModuleEncounters)).Post("/procedure-orders/{procID}/delete", encounterHandlers.DeleteProcedureOrderHandler)
			er.With(edit(auth.ModuleEncounters)).Post("/quick-add-procedure", encounterHandlers.QuickAddProcedureHandler)
			er.With(edit(auth.ModuleEncounters)).Post("/quick-add-lookup-value", encounterHandlers.QuickAddLookupValueHandler)
			er.With(edit(auth.ModuleEncounters)).Post("/complete", encounterHandlers.CompleteHandler)
			er.With(edit(auth.ModuleEncounters)).Post("/reopen", encounterHandlers.ReopenHandler)
			er.Get("/print", encounterHandlers.PrintHandler)
		})

		pr.Route("/billing", func(br chi.Router) {
			br.Use(view(auth.ModuleBilling))
			br.Get("/", billingHandlers.Index)
			br.With(edit(auth.ModuleBilling)).Get("/new", billingHandlers.NewInvoiceForm)
			br.With(edit(auth.ModuleBilling)).Post("/new", billingHandlers.CreateInvoice)
			br.With(edit(auth.ModuleBilling)).Get("/quick", billingHandlers.NewQuickInvoiceForm)
			br.With(edit(auth.ModuleBilling)).Post("/quick", billingHandlers.CreateQuickInvoice)
			br.Get("/invoices/{id}", billingHandlers.ViewInvoice)
			br.Get("/invoices/{id}/print", billingHandlers.PrintInvoice)
			br.With(edit(auth.ModuleBilling)).Post("/invoices/{id}/payments", billingHandlers.RecordPayment)
			br.With(edit(auth.ModuleBilling)).Post("/invoices/{id}/cancel", billingHandlers.CancelInvoice)
		})
	})

	log.Printf("chikitsalaya listening on :%s", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, r); err != nil {
		log.Fatalf("server: %v", err)
	}
}
