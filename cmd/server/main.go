package main

import (
	"errors"
	"fmt"
	"log"
	"net/http"

	_ "file-converter/docs" // Import swagger docs
	"file-converter/internal/api"
	"file-converter/internal/auth"
	"file-converter/internal/converter" // NEW: for CheckExtractDependencies()
	_ "file-converter/internal/formatters"
    "file-converter/internal/tasks"
	httpSwagger "github.com/swaggo/http-swagger"
)

// @title File Converter API
// @version 1.0
// @description Utility Microservice for converting files and user authentication.
// @host localhost:8080
// @BasePath /
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description Type "Bearer" followed by a space and JWT token.
func main() {
	// ============================================================
	// Startup environment checks (non-fatal)
	// ============================================================
	//
	// The conversion engine shells out to a set of external binaries.
	// Some are essential to specific endpoints; others are optional
	// optimizations that the code silently skips when unavailable.
	//
	// We do NOT fail fast. Every endpoint falls back gracefully when
	// a tool is missing:
	//
	//   - /extract-text  falls back to no-OCR for images and scanned PDFs
	//   - /convert       falls back to native Go converters for office
	//                    document pairs (loses formatting but still works)
	//   - /convert-pdf-size  falls back to the quality-ladder rasterizer
	//   - /compress-image    falls back to Go stdlib jpeg/png encoders
	//
	// We log two consolidated warnings so the operator can see at a
	// glance which optimizations are unavailable on this host.
	if missing := converter.CheckExtractDependencies(); len(missing) > 0 {
		log.Printf(
			"[WARN] /extract-text missing runtime dependencies: %v — image and scanned-PDF extraction will be unavailable.",
			missing,
		)
	}
	if missing := converter.CheckBridgeDependencies(); len(missing) > 0 {
		log.Printf(
			"[WARN] Conversion toolchain incomplete: missing %v — affected conversions will fall back to lower-fidelity implementations.",
			missing,
		)
	}
	

	// API Server
	apiMux := http.NewServeMux()
	apiMux.HandleFunc("/convert", api.HandleConvert)
	apiMux.HandleFunc("/convert-measurement", api.HandleMeasurementConvert)
	apiMux.HandleFunc("/convert-time", api.HandleTimeConvert)
	apiMux.HandleFunc("/convert-railway", api.HandleRailwayConvert)
	apiMux.HandleFunc("/convert-numeral", api.HandleNumeralConvert)
	apiMux.HandleFunc("/calculate-bmi", api.HandleBMICalculate)
	apiMux.HandleFunc("/calculate-age", api.HandleAgeCalculate)
	apiMux.HandleFunc("/convert-pdf-size", api.ConvertPDFSize)
	// ⬅️ NEW: Image Compress (JPEG/PNG/WebP quality-ladder compressor)
	apiMux.HandleFunc("/compress-image", api.CompressImage)
	// ⬅️ NEW: Text Extraction (PDF + Image, with OCR fallback)
	apiMux.HandleFunc("/extract-text", api.HandleExtractText)

	// ⬅️ NEW: Percentage Calculator
	apiMux.HandleFunc("/calculate-percentage", api.HandlePercentageCalculate)

	// ⬅️ NEW: Loan EMI Calculator
	apiMux.HandleFunc("/calculate-emi", api.HandleEMICalculate)

	// ⬅️ NEW: Borrower-centric Loan Tenure Calculator
	apiMux.HandleFunc("/calculate-loan-tenure", api.HandleLoanTenureCalculate)
	apiMux.HandleFunc("/calculate-tax", api.HandleTaxCalculate)
	apiMux.HandleFunc("/calculate-gpa", api.HandleGPACalculate)
	apiMux.HandleFunc("/calculate-simple-interest", api.HandleSimpleInterestCalculate)
	apiMux.HandleFunc("/calculate-compound-interest", api.HandleCompoundInterestCalculate)
	apiMux.HandleFunc("/calculate-scientific", api.HandleScientificCalculate)
	apiMux.HandleFunc("/formats", api.HandleListFormats)
	apiMux.HandleFunc("/formats/", api.HandleFormatDetail)
	// ============================================================
	// Authentication Service & Endpoints
	// ============================================================
	cfg, err := auth.LoadConfig("config.json")
	switch {
	case errors.Is(err, auth.ErrConfigNotFound):
		// The config file was not found on ANY search path. The
		// returned cfg is DefaultConfig() (SMTP disabled), so the
		// rest of the server still boots — but we make it LOUD
		// because "no email is being sent" is a silent-only failure
		// otherwise.
		log.Printf("=========================================================")
		log.Printf("[AUTH-WARN] config.json was not found on any search path.")
		log.Printf("[AUTH-WARN] SMTP is DISABLED. Emails (temporary passwords,")
		log.Printf("[AUTH-WARN] task notifications) will be LOGGED TO STDOUT only.")
		log.Printf("[AUTH-WARN] To fix: create config.json in the project root")
		log.Printf("[AUTH-WARN] with 'smtp_email' and 'smtp_app_password' set.")
		log.Printf("=========================================================")
	case err != nil:
		// The file was found but is malformed JSON, or has some other
		// parse error. That's a real bug in the config — abort loudly.
		log.Fatalf("[AUTH-FATAL] config.json is malformed: %v", err)
	}

	userStore, err := auth.NewStore(cfg.GetUsersFilePath())
	if err != nil {
		log.Fatalf("Failed to initialize user store: %v", err)
	}

	emailSender := auth.NewEmailSender(cfg)
	if emailSender.IsConfigured() {
		log.Printf("[AUTH] Google SMTP Service is ACTIVE. Sender: <%s>, Host: %s:%s", cfg.SMTPEmail, cfg.SMTPHost, cfg.SMTPPort)
	} else {
		log.Printf("[AUTH-WARN] Google SMTP Service is DISABLED (smtp_email or smtp_app_password missing). Running in Development Fallback Mode (temporary passwords will be printed to console).")
	}

	authService := auth.NewAuthService(userStore, cfg, emailSender)
	authHandler := api.NewAuthHandler(authService, cfg)

	apiMux.HandleFunc("/auth/register", authHandler.Register)
	apiMux.HandleFunc("/auth/login", authHandler.Login)
	apiMux.HandleFunc("/auth/logout", authHandler.Logout)
	apiMux.HandleFunc("/auth/forgot-password", authHandler.ForgotPassword)
	apiMux.HandleFunc("/auth/reset-password", api.RequireAuth(cfg, authHandler.ResetPassword))
	apiMux.HandleFunc("/auth/change-password", api.RequireAuth(cfg, authHandler.ChangePassword))
	apiMux.HandleFunc("/auth/me", api.RequireAuth(cfg, authHandler.Me))

	// ⬅️ NEW: Tasks (phone-notes style) Service & Endpoints (Authenticated)
	tasksStore, err := tasks.NewStore(cfg.GetDataDir() + "/tasks.json")
	if err != nil {
		log.Fatalf("Failed to initialize tasks store: %v", err)
	}
	// Ownership migration notice: any task without a user_id predates
	// the user-scoping feature and will not appear in any user's list.
	// Log a single line so operators are not surprised.
	if orphans, oerr := tasksStore.CountOrphans(); oerr == nil && orphans > 0 {
		log.Printf("[tasks] %d legacy task(s) with no owner detected; they will not appear in any user's list", orphans)
	}
	tasksService := tasks.NewService(tasksStore)
	tasksHandler := api.NewTasksHandler(tasksService, emailSender) // ⬅️ NEW: pass the sender

	apiMux.HandleFunc("/tasks", api.RequireAuth(cfg, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			tasksHandler.List(w, r)
		case http.MethodPost:
			tasksHandler.Create(w, r)
		default:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusMethodNotAllowed)
			_, _ = w.Write([]byte(`{"error":"Method not allowed"}`))
		}
	}))

	apiMux.HandleFunc("/tasks/", api.RequireAuth(cfg, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			tasksHandler.Update(w, r)
		case http.MethodDelete:
			tasksHandler.Delete(w, r)
		default:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusMethodNotAllowed)
			_, _ = w.Write([]byte(`{"error":"Method not allowed"}`))
		}
	}))

	// ── Repair engine ─────────────────────────────────────────
	apiMux.HandleFunc("/repair", api.HandleRepair)

	apiMux.HandleFunc("/swagger/", httpSwagger.WrapHandler)

	// Serve Frontend UI on apiMux root as fallback so port 8080 serves both API and Web UI
	apiMux.Handle("/", http.FileServer(http.Dir("./frontend/dist")))

	// UI Server
	uiMux := http.NewServeMux()
	uiMux.Handle("/", http.FileServer(http.Dir("./frontend/dist")))

	fmt.Println("==================================================")
	fmt.Println(" Utility Microservice Starting...")
	fmt.Println("==================================================")
	fmt.Println("Backend API:")
	fmt.Println(" -> POST http://localhost:8080/convert               (Universal File Converter)")
	fmt.Println("     - Native:    txt ↔ csv ↔ json, image ↔ image")
	fmt.Println("     - LibreOffice: docx, pptx, rtf, odt, ods, odp, html → pdf")
	fmt.Println("     - LibreOffice: docx → txt/json, pdf → docx")
	fmt.Println("     - Lossless:  jpg, png, webp, tiff, bmp → pdf")
	fmt.Println(" -> POST http://localhost:8080/convert-pdf-size      (PDF Compress / Expand)")
	fmt.Println("     - Quality-ladder rasterizer")
	fmt.Println("     - Ghostscript /ebook post-pass (10-20% extra reduction)")
	fmt.Println("     - qpdf linearization for web streaming")
	fmt.Println(" -> POST http://localhost:8080/compress-image        (Image Compress: JPG/PNG/WebP)")
	fmt.Println("     - mozjpeg for JPEG (15-25% smaller than stdlib)")
	fmt.Println("     - pngquant + oxipng for PNG (40-70% smaller)")
	fmt.Println("     - cwebp for WebP")
	fmt.Println(" -> POST http://localhost:8080/extract-text          (Text Extraction: PDF + Image OCR)")
	fmt.Println(" -> POST http://localhost:8080/repair                (Rules-based file repair)")
	fmt.Println(" -> POST http://localhost:8080/convert-measurement   (Measurement Converter)")
	fmt.Println(" -> POST http://localhost:8080/convert-time          (Time Zone Converter)")
	fmt.Println(" -> POST http://localhost:8080/convert-railway       (Railway Time Converter)")
	fmt.Println(" -> POST http://localhost:8080/convert-numeral       (Numeral System Converter)")
	fmt.Println(" -> POST http://localhost:8080/calculate-bmi         (BMI Calculator)")
	fmt.Println(" -> POST http://localhost:8080/calculate-age         (Age Calculator)")
	fmt.Println(" -> POST http://localhost:8080/calculate-percentage  (Percentage Calculator)")
	fmt.Println(" -> POST http://localhost:8080/calculate-emi         (Loan EMI Calculator)")
	fmt.Println(" -> POST http://localhost:8080/calculate-loan-tenure (Loan Tenure Calculator)")
	fmt.Println(" -> POST http://localhost:8080/calculate-simple-interest   (Simple Interest)")
	fmt.Println(" -> POST http://localhost:8080/calculate-compound-interest (Compound Interest)")
	fmt.Println(" -> POST http://localhost:8080/calculate-tax         (Tax / VAT / GST Calculator)")
	fmt.Println(" -> POST http://localhost:8080/calculate-gpa         (GPA / CGPA Calculator)")
	fmt.Println(" -> POST http://localhost:8080/calculate-scientific  (Scientific Calculator)")
	fmt.Println("Authentication:")
	fmt.Println(" -> POST http://localhost:8080/auth/register         (User Registration)")
	fmt.Println(" -> POST http://localhost:8080/auth/login            (User Login - JWT)")
	fmt.Println(" -> POST http://localhost:8080/auth/logout           (User Logout)")
	fmt.Println(" -> POST http://localhost:8080/auth/forgot-password  (Forgot Password - Google SMTP)")
	fmt.Println(" -> POST http://localhost:8080/auth/reset-password   (Forced Password Reset)")
	fmt.Println(" -> POST http://localhost:8080/auth/change-password  (Change Password)")
	fmt.Println(" -> GET  http://localhost:8080/auth/me               (Current User Profile)")
		fmt.Println("Tasks (Authenticated):")
	fmt.Println(" -> GET    http://localhost:8080/tasks                   (List Tasks)")
	fmt.Println(" -> POST   http://localhost:8080/tasks                   (Create Task)")
	fmt.Println(" -> PUT    http://localhost:8080/tasks/{id}              (Update Task)")
	fmt.Println(" -> DELETE http://localhost:8080/tasks/{id}              (Delete Task)")
	fmt.Println("Documentation:")
	fmt.Println(" -> GET  http://localhost:8080/swagger/doc.json       (Swagger JSON)")
	fmt.Println(" -> GET  http://localhost:8080/swagger/               (Swagger UI)")
	fmt.Println(" -> GET  http://localhost:8080/formats                (Conversion Registry)")
	fmt.Println("Frontend UI:")
	fmt.Println(" -> GET  http://localhost:3000/")
	fmt.Println("==================================================")
	// Start API server in background
	go func() {
		if err := http.ListenAndServe(":8080", api.EnableCORS(apiMux)); err != nil {
			log.Fatalf("API Server failed to start: %v", err)
		}
	}()

	// Start UI server in a goroutine – it’s optional and should not crash the whole process
	go func() {
		if err := http.ListenAndServe(":3000", uiMux); err != nil {
			log.Printf("UI Server failed to start (non‑fatal): %v", err)
		}
	}()

	// Block forever so the program stays alive after both goroutines start
	select {}
}
