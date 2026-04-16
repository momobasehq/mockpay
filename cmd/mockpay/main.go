package main

import (
	"log"
	"os"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/logger"
	"github.com/gofiber/fiber/v3/middleware/recover"

	"github.com/momobasehq/mockpay/internal/airtel"
	"github.com/momobasehq/mockpay/internal/mtn"
	"github.com/momobasehq/mockpay/internal/sim"
	"github.com/momobasehq/mockpay/internal/store"
)

func main() {
	app := fiber.New(fiber.Config{
		AppName:      "MockPay",
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		ErrorHandler: func(c fiber.Ctx, err error) error {
			code := fiber.StatusInternalServerError
			if e, ok := err.(*fiber.Error); ok {
				code = e.Code
			}
			return c.Status(code).JSON(fiber.Map{"error": err.Error()})
		},
	})

	app.Use(logger.New(logger.Config{
		Format: "[${time}] ${status} ${latency} ${method} ${path}\n",
	}))
	app.Use(recover.New())

	mtnStore := store.NewMTNStore()
	airtelStore := store.NewAirtelStore()

	// Provider mock routes
	mtn.RegisterRoutes(app.Group("/mtn"), mtnStore)
	airtel.RegisterRoutes(app.Group("/airtel"), airtelStore)

	// -----------------------------------------------------------------------
	// Admin routes — for local dev tooling only, not part of provider APIs
	// -----------------------------------------------------------------------
	sim.RegisterRoutes(app.Group("/admin"), mtnStore, airtelStore)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("🚀  Mock payment server listening on :%s", port)
	log.Printf("    MTN routes  → http://localhost:%s/mtn/...", port)
	log.Printf("    Airtel routes → http://localhost:%s/airtel/...", port)
	log.Printf("    Admin panel → http://localhost:%s/admin/state", port)
	log.Printf("    Default MTN creds: mock-api-user / mock-api-key")
	log.Printf("    Default MTN OAPI key: mock-oapi-subscription-key (Ocp-Apim-Subscription-Key header)")
	log.Printf("    Airtel: any client_id / client_secret accepted")

	if err := app.Listen(":" + port); err != nil {
		log.Fatal(err)
	}
}
