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
	admin := app.Group("/admin")

	// GET /admin/state — dump all in-memory transactions across both providers
	admin.Get("/state", func(c fiber.Ctx) error {
		fr, minMs, maxMs := sim.Global.Snapshot()
		return c.JSON(fiber.Map{
			"sim": fiber.Map{
				"failureRate": fr,
				"minDelayMs":  minMs,
				"maxDelayMs":  maxMs,
			},
			"mtn":    mtnStore.Dump(),
			"airtel": airtelStore.Dump(),
		})
	})

	// POST /admin/config — tune simulation behaviour at runtime
	//
	// Body: { "failureRate": 0.5, "minDelayMs": 100, "maxDelayMs": 2000 }
	admin.Post("/config", func(c fiber.Ctx) error {
		var body struct {
			FailureRate *float64 `json:"failureRate"`
			MinDelayMs  *int     `json:"minDelayMs"`
			MaxDelayMs  *int     `json:"maxDelayMs"`
		}
		if err := c.Bind().Body(&body); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid body"})
		}
		curRate, curMin, curMax := sim.Global.Snapshot()
		if body.FailureRate != nil {
			curRate = *body.FailureRate
		}
		if body.MinDelayMs != nil {
			curMin = *body.MinDelayMs
		}
		if body.MaxDelayMs != nil {
			curMax = *body.MaxDelayMs
		}
		sim.Global.Update(curRate, curMin, curMax)
		rate, minMs, maxMs := sim.Global.Snapshot()
		return c.JSON(fiber.Map{
			"message":     "Simulation config updated",
			"failureRate": rate,
			"minDelayMs":  minMs,
			"maxDelayMs":  maxMs,
		})
	})

	// DELETE /admin/reset — wipe all transactions and tokens (API users are preserved)
	admin.Delete("/reset", func(c fiber.Ctx) error {
		mtnStore.Reset()
		airtelStore.Reset()
		return c.JSON(fiber.Map{"message": "All transactions and tokens cleared"})
	})

	// GET /admin/ready — simple healthcheck
	admin.Get("/ready", func(c fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok"})
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("🚀  Mock payment server listening on :%s", port)
	log.Printf("    MTN routes  → http://localhost:%s/mtn/...", port)
	log.Printf("    Airtel routes → http://localhost:%s/airtel/...", port)
	log.Printf("    Admin panel → http://localhost:%s/admin/state", port)
	log.Printf("    Default MTN creds: mock-api-user / mock-api-key")
	log.Printf("    Airtel: any client_id / client_secret accepted")
	log.Fatal(app.Listen(":" + port))
}
